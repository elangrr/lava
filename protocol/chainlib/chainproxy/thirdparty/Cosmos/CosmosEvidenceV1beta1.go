package cosmos_thirdparty

import (
	"context"
	"encoding/json"

	pb_pkg "cosmossdk.io/api/cosmos/evidence/v1beta1"
	"github.com/golang/protobuf/proto"
	"github.com/lavanet/lava/utils"
)

type implementedCosmosEvidenceV1beta1 struct {
	pb_pkg.UnimplementedQueryServer
	cb func(ctx context.Context, method string, reqBody []byte) ([]byte, error)
}

// this line is used by grpc_scaffolder #implementedCosmosEvidenceV1beta1

func (is *implementedCosmosEvidenceV1beta1) AllEvidence(ctx context.Context, req *pb_pkg.QueryAllEvidenceRequest) (*pb_pkg.QueryAllEvidenceResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.evidence.v1beta1.Query/AllEvidence", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAllEvidenceResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

// this line is used by grpc_scaffolder #Methods
