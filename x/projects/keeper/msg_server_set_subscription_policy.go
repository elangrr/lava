package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lavanet/lava/x/projects/types"
)

func (k msgServer) SetSubscriptionPolicy(goCtx context.Context, msg *types.MsgSetSubscriptionPolicy) (*types.MsgSetSubscriptionPolicyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	policy := msg.GetPolicy()
	err := k.SetPolicy(ctx, msg.GetProjects(), &policy, msg.GetCreator(), types.SET_SUBSCRIPTION_POLICY)
	if err != nil {
		return nil, err
	}

	return &types.MsgSetSubscriptionPolicyResponse{}, nil
}
