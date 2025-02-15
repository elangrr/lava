package lavasession

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gogo/status"
	"google.golang.org/grpc/codes"
)

const (
	MaxConsecutiveConnectionAttempts                 = 10
	TimeoutForEstablishingAConnection                = 1 * time.Second
	MaxSessionsAllowedPerProvider                    = 1000 // Max number of sessions allowed per provider
	MaxAllowedBlockListedSessionPerProvider          = 3
	MaximumNumberOfFailuresAllowedPerConsumerSession = 3
	RelayNumberIncrement                             = 1
	DataReliabilitySessionId                         = 0 // data reliability session id is 0. we can change to more sessions later if needed.
	DataReliabilityRelayNumber                       = 1
	DataReliabilityCuSum                             = 0
	GeolocationFlag                                  = "geolocation"
	TendermintUnsubscribeAll                         = "unsubscribe_all"
	IndexNotFound                                    = -15
	AverageWorldLatency                              = 300 * time.Millisecond
	MinValidAddressesForBlockingProbing              = 2
	BACKOFF_TIME_ON_FAILURE                          = 3 * time.Second
)

var AvailabilityPercentage sdk.Dec = sdk.NewDecWithPrec(5, 2) // TODO move to params pairing
const (
	PercentileToCalculateLatency = 0.9
	MinProvidersForSync          = 0.6
	LatencyThresholdStatic       = 1 * time.Second
	LatencyThresholdSlope        = 1 * time.Millisecond
	StaleEpochDistance           = 3 // relays done 3 epochs back are ready to be rewarded

)

func IsSessionSyncLoss(err error) bool {
	code := status.Code(err)
	return code == codes.Code(SessionOutOfSyncError.ABCICode())
}
