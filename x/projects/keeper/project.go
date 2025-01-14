package keeper

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lavanet/lava/utils"
	"github.com/lavanet/lava/x/projects/types"
)

func (k Keeper) GetProjectForBlock(ctx sdk.Context, projectID string, blockHeight uint64) (types.Project, error) {
	var project types.Project

	if found := k.projectsFS.FindEntry(ctx, projectID, blockHeight, &project); !found {
		return project, utils.LavaError(ctx, ctx.Logger(), "GetProjectForBlock_not_found", map[string]string{"project": projectID, "blockHeight": strconv.FormatUint(blockHeight, 10)}, "project not found")
	}

	return project, nil
}

func (k Keeper) GetProjectDeveloperData(ctx sdk.Context, developerKey string, blockHeight uint64) (types.ProtoDeveloperData, error) {
	var projectDeveloperData types.ProtoDeveloperData
	if found := k.developerKeysFS.FindEntry(ctx, developerKey, blockHeight, &projectDeveloperData); !found {
		return types.ProtoDeveloperData{}, fmt.Errorf("GetProjectIDForDeveloper_invalid_key, the requesting key is not registered to a project, developer: %s", developerKey)
	}
	return projectDeveloperData, nil
}

func (k Keeper) GetProjectForDeveloper(ctx sdk.Context, developerKey string, blockHeight uint64) (proj types.Project, vrfpk string, errRet error) {
	var project types.Project
	projectDeveloperData, err := k.GetProjectDeveloperData(ctx, developerKey, blockHeight)
	if err != nil {
		return project, "", err
	}

	if found := k.projectsFS.FindEntry(ctx, projectDeveloperData.ProjectID, blockHeight, &project); !found {
		return project, "", utils.LavaError(ctx, ctx.Logger(), "GetProjectForDeveloper_project_not_found", map[string]string{"developer": developerKey, "project": projectDeveloperData.ProjectID}, "the developers project was not found")
	}

	return project, projectDeveloperData.Vrfpk, nil
}

func (k Keeper) AddKeysToProject(ctx sdk.Context, projectID string, adminKey string, projectKeys []types.ProjectKey) error {
	var project types.Project
	if found := k.projectsFS.FindEntry(ctx, projectID, uint64(ctx.BlockHeight()), &project); !found {
		return utils.LavaError(ctx, ctx.Logger(), "AddProjectKeys_project_not_found", map[string]string{"project": projectID}, "project id not found")
	}

	// check if the admin key is valid
	if !project.IsAdminKey(adminKey) {
		return utils.LavaError(ctx, ctx.Logger(), "AddProjectKeys_not_admin", map[string]string{"project": projectID}, "the requesting key is not admin key")
	}

	for _, projectKey := range projectKeys {
		err := k.RegisterKey(ctx, projectKey, &project, uint64(ctx.BlockHeight()))
		if err != nil {
			return utils.LavaError(ctx, ctx.Logger(), "AddProjectKeys_register_key_failed", map[string]string{"err": err.Error(), "project": projectID, "projectKeyAddress": projectKey.GetKey(), "projectKeyTypes": string(projectKey.GetTypes())}, "failed to register key")
		}
	}

	return k.projectsFS.AppendEntry(ctx, projectID, uint64(ctx.BlockHeight()), &project)
}

func (k Keeper) ChargeComputeUnitsToProject(ctx sdk.Context, project types.Project, cu uint64) (err error) {
	project.UsedCu += cu
	return k.projectsFS.ModifyEntry(ctx, project.Index, uint64(ctx.BlockHeight()), &project)
}

func (k Keeper) SetPolicy(ctx sdk.Context, projectIDs []string, policy *types.Policy, key string, setPolicyEnum types.SetPolicyEnum) error {
	for _, projectID := range projectIDs {
		project, err := k.GetProjectForBlock(ctx, projectID, uint64(ctx.BlockHeight()))
		if err != nil {
			return utils.LavaError(ctx, ctx.Logger(), "SetPolicy_project_not_found", map[string]string{"project": projectID}, "project id not found")
		}
		// for admin policy - check if the key is an address of a project admin.
		// Note, the subscription key is also considered an admin key
		if setPolicyEnum == types.SET_ADMIN_POLICY {
			if !project.IsAdminKey(key) {
				return utils.LavaError(ctx, ctx.Logger(), "SetPolicy_not_admin", map[string]string{"project": projectID, "key": key}, "cannot set admin policy because the requesting key is not admin key")
			} else {
				project.AdminPolicy = policy
			}
		} else if setPolicyEnum == types.SET_SUBSCRIPTION_POLICY {
			// for subscription policy - check if the key is an address of the project's subscription consumer
			if key != project.GetSubscription() {
				return utils.LavaError(ctx, ctx.Logger(), "SetPolicy_not_subscription_consumer", map[string]string{"project": projectID, "key": key}, "cannot set subscription policy because the requesting key is not subscription consumer key")
			} else {
				project.SubscriptionPolicy = policy
			}
		}

		nextEpoch, err := k.epochStorageKeeper.GetNextEpoch(ctx, uint64(ctx.BlockHeight()))
		if err != nil {
			return utils.LavaError(ctx, k.Logger(ctx), "SetPolicy_cant_get_next_epoch", map[string]string{"block": strconv.FormatUint(uint64(ctx.BlockHeight()), 10)}, "can't get next epoch")
		}
		err = k.projectsFS.AppendEntry(ctx, projectID, nextEpoch, &project)
		if err != nil {
			return err
		}
	}

	return nil
}
