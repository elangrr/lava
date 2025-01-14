package rpcconsumer

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/coniks-sys/coniks-go/crypto/vrf"
	"github.com/lavanet/lava/protocol/chainlib"
	"github.com/lavanet/lava/protocol/common"
	"github.com/lavanet/lava/protocol/lavaprotocol"
	"github.com/lavanet/lava/protocol/lavasession"
	"github.com/lavanet/lava/protocol/metrics"
	"github.com/lavanet/lava/protocol/performance"
	"github.com/lavanet/lava/utils"
	conflicttypes "github.com/lavanet/lava/x/conflict/types"
	pairingtypes "github.com/lavanet/lava/x/pairing/types"
	spectypes "github.com/lavanet/lava/x/spec/types"
)

const (
	MaxRelayRetries = 4
)

// implements Relay Sender interfaced and uses an ChainListener to get it called
type RPCConsumerServer struct {
	chainParser            chainlib.ChainParser
	consumerSessionManager *lavasession.ConsumerSessionManager
	listenEndpoint         *lavasession.RPCEndpoint
	rpcConsumerLogs        *common.RPCConsumerLogs
	cache                  *performance.Cache
	privKey                *btcec.PrivateKey
	consumerTxSender       ConsumerTxSender
	requiredResponses      int
	finalizationConsensus  *lavaprotocol.FinalizationConsensus
	VrfSk                  vrf.PrivateKey
	lavaChainID            string
}

type ConsumerTxSender interface {
	TxConflictDetection(ctx context.Context, finalizationConflict *conflicttypes.FinalizationConflict, responseConflict *conflicttypes.ResponseConflict, sameProviderConflict *conflicttypes.FinalizationConflict) error
}

func (rpccs *RPCConsumerServer) ServeRPCRequests(ctx context.Context, listenEndpoint *lavasession.RPCEndpoint,
	consumerStateTracker ConsumerStateTrackerInf,
	chainParser chainlib.ChainParser,
	finalizationConsensus *lavaprotocol.FinalizationConsensus,
	consumerSessionManager *lavasession.ConsumerSessionManager,
	requiredResponses int,
	privKey *btcec.PrivateKey,
	vrfSk vrf.PrivateKey,
	lavaChainID string,
	cache *performance.Cache, // optional
) (err error) {
	rpccs.consumerSessionManager = consumerSessionManager
	rpccs.listenEndpoint = listenEndpoint
	rpccs.cache = cache
	rpccs.consumerTxSender = consumerStateTracker
	rpccs.requiredResponses = requiredResponses
	rpccs.VrfSk = vrfSk
	pLogs, err := common.NewRPCConsumerLogs()
	if err != nil {
		utils.LavaFormatFatal("failed creating RPCConsumer logs", err)
	}
	rpccs.lavaChainID = lavaChainID
	rpccs.rpcConsumerLogs = pLogs
	rpccs.privKey = privKey
	rpccs.chainParser = chainParser
	rpccs.finalizationConsensus = finalizationConsensus
	chainListener, err := chainlib.NewChainListener(ctx, listenEndpoint, rpccs, pLogs)
	if err != nil {
		return err
	}
	go chainListener.Serve(ctx)
	return nil
}

func (rpccs *RPCConsumerServer) SendRelay(
	ctx context.Context,
	url string,
	req string,
	connectionType string,
	dappID string,
	analytics *metrics.RelayMetrics,
) (relayReply *pairingtypes.RelayReply, relayServer *pairingtypes.Relayer_RelaySubscribeClient, errRet error) {
	// gets the relay request data from the ChainListener
	// parses the request into an APIMessage, and validating it corresponds to the spec currently in use
	// construct the common data for a relay message, common data is identical across multiple sends and data reliability
	// sends a relay message to a provider
	// compares the result with other providers if defined so
	// compares the response with other consumer wallets if defined so
	// asynchronously sends data reliability if necessary
	relaySentTime := time.Now()
	chainMessage, err := rpccs.chainParser.ParseMsg(url, []byte(req), connectionType)
	if err != nil {
		return nil, nil, err
	}
	// Unmarshal request
	unwantedProviders := map[string]struct{}{}

	// do this in a loop with retry attempts, configurable via a flag, limited by the number of providers in CSM
	relayRequestData := lavaprotocol.NewRelayData(ctx, connectionType, url, []byte(req), chainMessage.RequestedBlock(), rpccs.listenEndpoint.ApiInterface)
	relayResults := []*lavaprotocol.RelayResult{}
	relayErrors := []error{}
	blockOnSyncLoss := true
	for retries := 0; retries < MaxRelayRetries; retries++ {
		// TODO: make this async between different providers
		relayResult, err := rpccs.sendRelayToProvider(ctx, chainMessage, relayRequestData, dappID, &unwantedProviders)
		if relayResult.ProviderAddress != "" {
			if blockOnSyncLoss && lavasession.IsSessionSyncLoss(err) {
				utils.LavaFormatDebug("Identified SyncLoss in provider, not removing it from list for another attempt", utils.Attribute{Key: "address", Value: relayResult.ProviderAddress})
				blockOnSyncLoss = false // on the first sync loss no need to block the provider. give it another chance
			} else {
				unwantedProviders[relayResult.ProviderAddress] = struct{}{}
			}
		}
		if err != nil {
			relayErrors = append(relayErrors, err)
			if lavasession.PairingListEmptyError.Is(err) {
				// if we ran out of pairings because unwantedProviders is too long or validProviders is too short, continue to reply handling code
				break
			}
			// decide if we should break here if its something retry won't solve
			utils.LavaFormatDebug("could not send relay to provider", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "error", Value: err.Error()})
			continue
		}
		relayResults = append(relayResults, relayResult)
		if len(relayResults) >= rpccs.requiredResponses {
			break
		}
		// future requests need to ask for the same block height to get consensus on the reply
		relayRequestData.RequestBlock = relayResult.Request.RelayData.RequestBlock
	}

	enabled, dataReliabilityThreshold := rpccs.chainParser.DataReliabilityParams()
	if enabled {
		for _, relayResult := range relayResults {
			// new context is needed for data reliability as some clients cancel the context they provide when the relay returns
			// as data reliability happens in a go routine it will continue while the response returns.
			guid, found := utils.GetUniqueIdentifier(ctx)
			dataReliabilityContext := context.Background()
			if found {
				dataReliabilityContext = utils.WithUniqueIdentifier(dataReliabilityContext, guid)
			}
			go rpccs.sendDataReliabilityRelayIfApplicable(dataReliabilityContext, relayResult, chainMessage, dataReliabilityThreshold) // runs asynchronously
		}
	}

	// TODO: secure, go over relay results to find discrepancies and choose majority, or trigger a second wallet relay
	if len(relayResults) == 0 {
		return nil, nil, utils.LavaFormatError("Failed all retries", nil, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "errors", Value: relayErrors})
	} else if len(relayErrors) > 0 {
		utils.LavaFormatDebug("relay succeeded but had some errors", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "errors", Value: relayErrors})
	}
	var returnedResult *lavaprotocol.RelayResult
	for _, iteratedResult := range relayResults {
		// TODO: go over rpccs.requiredResponses and get majority
		returnedResult = iteratedResult
	}

	if analytics != nil {
		currentLatency := time.Since(relaySentTime)
		analytics.Latency = currentLatency.Milliseconds()
		analytics.ComputeUnits = returnedResult.Request.RelaySession.CuSum
	}

	return returnedResult.Reply, returnedResult.ReplyServer, nil
}

func (rpccs *RPCConsumerServer) sendRelayToProvider(
	ctx context.Context,
	chainMessage chainlib.ChainMessage,
	relayRequestData *pairingtypes.RelayPrivateData,
	dappID string,
	unwantedProviders *map[string]struct{},
) (relayResult *lavaprotocol.RelayResult, errRet error) {
	// get a session for the relay from the ConsumerSessionManager
	// construct a relay message with lavaprotocol package, include QoS and jail providers
	// sign the relay message with the lavaprotocol package
	// send the relay message
	// handle the response verification with the lavaprotocol package
	// handle data reliability provider finalization data with the lavaprotocol package
	// if necessary send detection tx for breach of data reliability provider finalization data
	// handle data reliability hashes consensus checks with the lavaprotocol package
	// if necessary send detection tx for hashes consensus mismatch
	// handle QoS updates
	// in case connection totally fails, update unresponsive providers in ConsumerSessionManager

	isSubscription := chainMessage.GetInterface().Category.Subscription

	// Get Session. we get session here so we can use the epoch in the callbacks
	singleConsumerSession, epoch, providerPublicAddress, reportedProviders, err := rpccs.consumerSessionManager.GetSession(ctx, chainMessage.GetServiceApi().ComputeUnits, *unwantedProviders)
	relayResult = &lavaprotocol.RelayResult{ProviderAddress: providerPublicAddress, Finalized: false}
	if err != nil {
		return relayResult, err
	}
	privKey := rpccs.privKey
	chainID := rpccs.listenEndpoint.ChainID
	lavaChainID := rpccs.lavaChainID
	relayRequest, err := lavaprotocol.ConstructRelayRequest(ctx, privKey, lavaChainID, chainID, relayRequestData, providerPublicAddress, singleConsumerSession, int64(epoch), reportedProviders)
	if err != nil {
		return relayResult, err
	}
	relayResult.Request = relayRequest
	endpointClient := *singleConsumerSession.Endpoint.Client

	if isSubscription {
		return rpccs.relaySubscriptionInner(ctx, endpointClient, singleConsumerSession, relayResult)
	}

	// try using cache before sending relay
	var reply *pairingtypes.RelayReply

	reply, err = rpccs.cache.GetEntry(ctx, relayRequest, chainMessage.GetInterface().Interface, nil, chainID, false) // caching in the portal doesn't care about hashes, and we don't have data on finalization yet
	if err == nil && reply != nil {
		// Info was fetched from cache, so we don't need to change the state
		// so we can return here, no need to update anything and calculate as this info was fetched from the cache
		relayResult.Reply = reply
		err = rpccs.consumerSessionManager.OnSessionUnUsed(singleConsumerSession)
		return relayResult, err
	}

	// cache failed, move on to regular relay
	if performance.NotConnectedError.Is(err) {
		utils.LavaFormatError("cache not connected", err)
	}

	extraRelayTimeout := time.Duration(0)
	if chainMessage.GetInterface().Category.HangingApi {
		_, extraRelayTimeout, _, _ = rpccs.chainParser.ChainBlockStats()
	}
	relayTimeout := extraRelayTimeout + lavaprotocol.GetTimePerCu(singleConsumerSession.LatestRelayCu) + lavasession.AverageWorldLatency
	relayResult, relayLatency, err, backoff := rpccs.relayInner(ctx, singleConsumerSession, relayResult, relayTimeout)
	if err != nil {
		failRelaySession := func(origErr error, backoff_ bool) {
			backOffDuration := 0 * time.Second
			if backoff_ {
				backOffDuration = lavasession.BACKOFF_TIME_ON_FAILURE
			}
			time.Sleep(backOffDuration) // sleep before releasing this singleConsumerSession
			// relay failed need to fail the session advancement
			errReport := rpccs.consumerSessionManager.OnSessionFailure(singleConsumerSession, err)
			if errReport != nil {
				utils.LavaFormatError("failed relay onSessionFailure errored", errReport, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "original error", Value: err.Error()})
			}
		}
		go failRelaySession(err, backoff)
		return relayResult, err
	}
	// get here only if performed a regular relay successfully
	expectedBH, numOfProviders := rpccs.finalizationConsensus.ExpectedBlockHeight(rpccs.chainParser)
	pairingAddressesLen := rpccs.consumerSessionManager.GetAtomicPairingAddressesLength()
	latestBlock := relayResult.Reply.LatestBlock
	err = rpccs.consumerSessionManager.OnSessionDone(singleConsumerSession, epoch, latestBlock, chainMessage.GetServiceApi().ComputeUnits, relayLatency, singleConsumerSession.CalculateExpectedLatency(relayTimeout), expectedBH, numOfProviders, pairingAddressesLen) // session done successfully

	// set cache in a non blocking call
	go func() {
		new_ctx := context.Background()
		new_ctx, cancel := context.WithTimeout(new_ctx, chainlib.DataReliabilityTimeoutIncrease)
		defer cancel()
		err2 := rpccs.cache.SetEntry(new_ctx, relayRequest, chainMessage.GetInterface().Interface, nil, chainID, dappID, relayResult.Reply, relayResult.Finalized) // caching in the portal doesn't care about hashes
		if err2 != nil && !performance.NotInitialisedError.Is(err2) {
			utils.LavaFormatWarning("error updating cache with new entry", err2)
		}
	}()
	return relayResult, err
}

func (rpccs *RPCConsumerServer) relayInner(ctx context.Context, singleConsumerSession *lavasession.SingleConsumerSession, relayResult *lavaprotocol.RelayResult, relayTimeout time.Duration) (relayResultRet *lavaprotocol.RelayResult, relayLatency time.Duration, err error, needsBackoff bool) {
	existingSessionLatestBlock := singleConsumerSession.LatestBlock // we read it now because singleConsumerSession is locked, and later it's not
	endpointClient := *singleConsumerSession.Endpoint.Client
	providerPublicAddress := relayResult.ProviderAddress
	relayRequest := relayResult.Request
	callRelay := func() (reply *pairingtypes.RelayReply, relayLatency time.Duration, err error, backoff bool) {
		relaySentTime := time.Now()
		connectCtx, connectCtxCancel := context.WithTimeout(ctx, relayTimeout)
		defer connectCtxCancel()
		reply, err = endpointClient.Relay(connectCtx, relayRequest)
		relayLatency = time.Since(relaySentTime)
		if err != nil {
			backoff := false
			if errors.Is(connectCtx.Err(), context.DeadlineExceeded) {
				backoff = true
			}
			return reply, 0, err, backoff
		}
		return reply, relayLatency, nil, false
	}
	reply, relayLatency, err, backoff := callRelay()
	if err != nil {
		return relayResult, 0, err, backoff
	}
	relayResult.Reply = reply
	lavaprotocol.UpdateRequestedBlock(relayRequest.RelayData, reply) // update relay request requestedBlock to the provided one in case it was arbitrary
	_, _, blockDistanceForFinalizedData, _ := rpccs.chainParser.ChainBlockStats()
	finalized := spectypes.IsFinalizedBlock(relayRequest.RelayData.RequestBlock, reply.LatestBlock, blockDistanceForFinalizedData)
	err = lavaprotocol.VerifyRelayReply(reply, relayRequest, providerPublicAddress)
	if err != nil {
		return relayResult, 0, err, false
	}

	// TODO: response data sanity, check its under an expected format add that format to spec
	enabled, _ := rpccs.chainParser.DataReliabilityParams()
	if enabled {
		// TODO: DETECTION instead of existingSessionLatestBlock, we need proof of last reply to send the previous reply and the current reply
		finalizedBlocks, finalizationConflict, err := lavaprotocol.VerifyFinalizationData(reply, relayRequest, providerPublicAddress, existingSessionLatestBlock, blockDistanceForFinalizedData)
		if err != nil {
			if lavaprotocol.ProviderFinzalizationDataAccountabilityError.Is(err) && finalizationConflict != nil {
				go rpccs.consumerTxSender.TxConflictDetection(ctx, finalizationConflict, nil, nil)
			}
			return relayResult, 0, err, false
		}

		finalizationConflict, err = rpccs.finalizationConsensus.UpdateFinalizedHashes(int64(blockDistanceForFinalizedData), providerPublicAddress, reply.LatestBlock, finalizedBlocks, relayRequest.RelaySession, reply)
		if err != nil {
			go rpccs.consumerTxSender.TxConflictDetection(ctx, finalizationConflict, nil, nil)
			return relayResult, 0, err, false
		}
	}
	relayResult.Finalized = finalized
	return relayResult, relayLatency, nil, false
}

func (rpccs *RPCConsumerServer) relaySubscriptionInner(ctx context.Context, endpointClient pairingtypes.RelayerClient, singleConsumerSession *lavasession.SingleConsumerSession, relayResult *lavaprotocol.RelayResult) (relayResultRet *lavaprotocol.RelayResult, err error) {
	// relaySentTime := time.Now()
	replyServer, err := endpointClient.RelaySubscribe(ctx, relayResult.Request)
	// relayLatency := time.Since(relaySentTime) // TODO: use subscription QoS
	if err != nil {
		errReport := rpccs.consumerSessionManager.OnSessionFailure(singleConsumerSession, err)
		if errReport != nil {
			return relayResult, utils.LavaFormatError("subscribe relay failed onSessionFailure errored", errReport, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "original error", Value: err.Error()})
		}
		return relayResult, err
	}
	// TODO: need to check that if provider fails and returns error, this is reflected here and we run onSessionDone
	// my thoughts are that this fails if the grpc fails not if the provider fails, and if the provider returns an error this is reflected by the Recv function on the chainListener calling us here
	// and this is too late
	relayResult.ReplyServer = &replyServer
	err = rpccs.consumerSessionManager.OnSessionDoneIncreaseCUOnly(singleConsumerSession)
	return relayResult, err
}

func (rpccs *RPCConsumerServer) sendDataReliabilityRelayIfApplicable(ctx context.Context, relayResult *lavaprotocol.RelayResult, chainMessage chainlib.ChainMessage, dataReliabilityThreshold uint32) error {
	// Data reliability:
	// handle data reliability VRF random value check with the lavaprotocol package
	// asynchronous: if applicable, get a data reliability session from ConsumerSessionManager
	// construct a data reliability relay message with lavaprotocol package
	// sign the data reliability relay message with the lavaprotocol package
	// send the data reliability relay message with the lavaprotocol grpc service
	// check validity of the data reliability response with the lavaprotocol package
	// compare results for both relays, if there is a difference send a detection tx with both requests and both responses

	// validate relayResult is not nil
	if relayResult == nil || relayResult.Reply == nil || relayResult.Request == nil {
		return utils.LavaFormatError("sendDataReliabilityRelayIfApplicable relayResult nil check", nil, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "relayResult", Value: relayResult})
	}

	specCategory := chainMessage.GetInterface().Category
	if !specCategory.Deterministic || !relayResult.Finalized {
		return nil // disabled for this spec and requested block so no data reliability messages
	}
	var dataReliabilitySessions []*lavasession.DataReliabilitySession
	sessionEpoch := uint64(relayResult.Request.RelaySession.Epoch)
	providerPubAddress := relayResult.ProviderAddress
	// handle data reliability
	vrfRes0, vrfRes1 := utils.CalculateVrfOnRelay(relayResult.Request.RelayData, relayResult.Reply, rpccs.VrfSk, sessionEpoch)
	// get two indexesMap for data reliability.
	providersCount := uint32(rpccs.consumerSessionManager.GetAtomicPairingAddressesLength())
	indexesMap := lavaprotocol.DataReliabilityThresholdToSession([][]byte{vrfRes0, vrfRes1}, []bool{false, true}, dataReliabilityThreshold, providersCount)
	utils.LavaFormatDebug("DataReliability Randomized Values", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "vrf0", Value: uint64(binary.LittleEndian.Uint32(vrfRes0))}, utils.Attribute{Key: "vrf1", Value: uint64(binary.LittleEndian.Uint32(vrfRes1))}, utils.Attribute{Key: "decisionMap", Value: indexesMap})
	for idxExtract, uniqueIdentifier := range indexesMap { // go over each unique index and get a session.
		// the key in the indexesMap are unique indexes to fetch from consumerSessionManager
		dataReliabilityConsumerSession, providerPublicAddress, epoch, err := rpccs.consumerSessionManager.GetDataReliabilitySession(ctx, providerPubAddress, idxExtract, sessionEpoch)
		if err != nil {
			if lavasession.DataReliabilityIndexRequestedIsOriginalProviderError.Is(err) {
				// index belongs to original provider, nothing is wrong here, print info and continue
				utils.LavaFormatInfo("DataReliability: Trying to get the same provider index as original request", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "provider", Value: providerPubAddress}, utils.Attribute{Key: "Index", Value: idxExtract})
			} else if lavasession.DataReliabilityAlreadySentThisEpochError.Is(err) {
				utils.LavaFormatInfo("DataReliability: Already Sent Data Reliability This Epoch To This Provider.", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "Provider", Value: providerPubAddress}, utils.Attribute{Key: "Epoch", Value: epoch})
			} else if lavasession.DataReliabilityEpochMismatchError.Is(err) {
				utils.LavaFormatInfo("DataReliability: Epoch changed cannot send data reliability", utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "original_epoch", Value: sessionEpoch}, utils.Attribute{Key: "data_reliability_epoch", Value: epoch})
				// if epoch changed, we can stop trying to get data reliability sessions
				break
			} else {
				utils.LavaFormatError("GetDataReliabilitySession", err, utils.Attribute{Key: "GUID", Value: ctx})
			}
			continue // if got an error continue to next index.
		}
		dataReliabilitySessions = append(dataReliabilitySessions, &lavasession.DataReliabilitySession{
			SingleConsumerSession: dataReliabilityConsumerSession,
			Epoch:                 epoch,
			ProviderPublicAddress: providerPublicAddress,
			UniqueIdentifier:      uniqueIdentifier,
		})
	}

	sendReliabilityRelay := func(singleConsumerSession *lavasession.SingleConsumerSession, providerAddress string, differentiator bool, epoch int64) (reliabilityResult *lavaprotocol.RelayResult, err error) {
		vrf_res, vrf_proof := utils.ProveVrfOnRelay(relayResult.Request.RelayData, relayResult.Reply, rpccs.VrfSk, differentiator, sessionEpoch)
		// calculated from query body anyway, but we will use this on payment
		// calculated in cb_send_reliability
		vrfData := lavaprotocol.NewVRFData(differentiator, vrf_res, vrf_proof, relayResult.Request, relayResult.Reply)
		reportedProviders, err := rpccs.consumerSessionManager.GetReportedProviders(uint64(epoch))
		if err != nil {
			reportedProviders = nil
			utils.LavaFormatError("failed reading reported providers for epoch", err, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "epoch", Value: epoch})
		}
		reliabilityRequest, err := lavaprotocol.ConstructDataReliabilityRelayRequest(ctx, rpccs.lavaChainID, vrfData, rpccs.privKey, rpccs.listenEndpoint.ChainID, relayResult.Request.RelayData, providerAddress, epoch, reportedProviders, singleConsumerSession.RelayNum)
		if err != nil {
			return nil, utils.LavaFormatError("failed creating data reliability relay", err, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "relayRequestData", Value: relayResult.Request.RelayData})
		}
		relayResult = &lavaprotocol.RelayResult{Request: reliabilityRequest, ProviderAddress: providerAddress, Finalized: false}
		relayTimeout := lavaprotocol.GetTimePerCu(singleConsumerSession.LatestRelayCu) + lavasession.AverageWorldLatency + chainlib.DataReliabilityTimeoutIncrease
		relayResult, dataReliabilityLatency, err, backoff := rpccs.relayInner(ctx, singleConsumerSession, relayResult, relayTimeout)
		if err != nil {
			failRelaySession := func(origErr error, backoff_ bool) {
				backOffDuration := 0 * time.Second
				if backoff_ {
					backOffDuration = lavasession.BACKOFF_TIME_ON_FAILURE
				}
				time.Sleep(backOffDuration) // sleep before releasing this singleConsumerSession
				// relay failed need to fail the session advancement
				errReport := rpccs.consumerSessionManager.OnDataReliabilitySessionFailure(singleConsumerSession, err)
				if errReport != nil {
					utils.LavaFormatError("OnDataReliabilitySessionFailure Error", errReport, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "sendReliabilityError", Value: err.Error()})
				}
			}
			go failRelaySession(err, backoff)
			return nil, utils.LavaFormatError("sendReliabilityRelay Could not get reply to reliability relay from provider", err, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "Address", Value: providerAddress})
		}

		expectedBH, numOfProviders := rpccs.finalizationConsensus.ExpectedBlockHeight(rpccs.chainParser)
		err = rpccs.consumerSessionManager.OnDataReliabilitySessionDone(singleConsumerSession, relayResult.Reply.LatestBlock, singleConsumerSession.LatestRelayCu, dataReliabilityLatency, singleConsumerSession.CalculateExpectedLatency(relayTimeout), expectedBH, numOfProviders, uint64(providersCount))
		return relayResult, err
	}

	checkReliability := func() {
		numberOfReliabilitySessions := len(dataReliabilitySessions)
		if numberOfReliabilitySessions > lavaprotocol.SupportedNumberOfVRFs {
			utils.LavaFormatError("Trying to use DataReliability with more than two vrf sessions, currently not supported", nil, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "number_of_DataReliabilitySessions", Value: strconv.Itoa(numberOfReliabilitySessions)})
			return
		} else if numberOfReliabilitySessions == 0 {
			return
		}
		// apply first request and reply to dataReliabilityVerifications

		dataReliabilityVerifications := make([]*lavaprotocol.RelayResult, 0)

		for _, dataReliabilitySession := range dataReliabilitySessions {
			reliabilityResult, err := sendReliabilityRelay(dataReliabilitySession.SingleConsumerSession, dataReliabilitySession.ProviderPublicAddress, dataReliabilitySession.UniqueIdentifier, int64(dataReliabilitySession.Epoch))
			if err == nil && reliabilityResult.Reply != nil {
				dataReliabilityVerifications = append(dataReliabilityVerifications,
					&lavaprotocol.RelayResult{
						Reply:           reliabilityResult.Reply,
						Request:         reliabilityResult.Request,
						ProviderAddress: dataReliabilitySession.ProviderPublicAddress,
					})
			} else {
				utils.LavaFormatWarning("failed data reliability relay", err, utils.Attribute{Key: "GUID", Value: ctx})
			}
		}
		if len(dataReliabilityVerifications) > 0 {
			report, conflicts := lavaprotocol.VerifyReliabilityResults(relayResult, dataReliabilityVerifications, numberOfReliabilitySessions)
			if report {
				for _, conflict := range conflicts {
					err := rpccs.consumerTxSender.TxConflictDetection(ctx, nil, conflict, nil)
					if err != nil {
						utils.LavaFormatError("could not send detection Transaction", err, utils.Attribute{Key: "GUID", Value: ctx}, utils.Attribute{Key: "conflict", Value: conflict})
					}
				}
			}
			// detectionMessage = conflicttypes.NewMsgDetection(consumerAddress, nil, &responseConflict, nil)
		}
	}
	checkReliability()
	return nil
}
