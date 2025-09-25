// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabUsersBundle struct{}

func NewGitlabUsers() types.Bundle {
	return &GitlabUsersBundle{}
}

func (b *GitlabUsersBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	// Manual actions
	case "createServiceAccountUser":
		return b.RunCreateServiceAccountUser(ctx, task, credential)
	// Auto-generated actions
	case "activateUser":
		return b.RunActivateUser(ctx, task, credential)
	case "addEmail":
		return b.RunAddEmail(ctx, task, credential)
	case "addEmailForUser":
		return b.RunAddEmailForUser(ctx, task, credential)
	case "addGPGKey":
		return b.RunAddGPGKey(ctx, task, credential)
	case "addGPGKeyForUser":
		return b.RunAddGPGKeyForUser(ctx, task, credential)
	case "addSSHKey":
		return b.RunAddSSHKey(ctx, task, credential)
	case "addSSHKeyForUser":
		return b.RunAddSSHKeyForUser(ctx, task, credential)
	case "approveUser":
		return b.RunApproveUser(ctx, task, credential)
	case "banUser":
		return b.RunBanUser(ctx, task, credential)
	case "blockUser":
		return b.RunBlockUser(ctx, task, credential)
	case "createUser":
		return b.RunCreateUser(ctx, task, credential)
	case "createUserRunner":
		return b.RunCreateUserRunner(ctx, task, credential)
	case "currentUser":
		return b.RunCurrentUser(ctx, task, credential)
	case "currentUserStatus":
		return b.RunCurrentUserStatus(ctx, task, credential)
	case "deactivateUser":
		return b.RunDeactivateUser(ctx, task, credential)
	case "deleteEmail":
		return b.RunDeleteEmail(ctx, task, credential)
	case "deleteEmailForUser":
		return b.RunDeleteEmailForUser(ctx, task, credential)
	case "deleteGPGKey":
		return b.RunDeleteGPGKey(ctx, task, credential)
	case "deleteGPGKeyForUser":
		return b.RunDeleteGPGKeyForUser(ctx, task, credential)
	case "deleteSSHKey":
		return b.RunDeleteSSHKey(ctx, task, credential)
	case "deleteSSHKeyForUser":
		return b.RunDeleteSSHKeyForUser(ctx, task, credential)
	case "deleteUser":
		return b.RunDeleteUser(ctx, task, credential)
	case "disableTwoFactor":
		return b.RunDisableTwoFactor(ctx, task, credential)
	case "getAllImpersonationTokens":
		return b.RunGetAllImpersonationTokens(ctx, task, credential)
	case "getEmail":
		return b.RunGetEmail(ctx, task, credential)
	case "getGPGKey":
		return b.RunGetGPGKey(ctx, task, credential)
	case "getGPGKeyForUser":
		return b.RunGetGPGKeyForUser(ctx, task, credential)
	case "getImpersonationToken":
		return b.RunGetImpersonationToken(ctx, task, credential)
	case "getSSHKey":
		return b.RunGetSSHKey(ctx, task, credential)
	case "getSSHKeyForUser":
		return b.RunGetSSHKeyForUser(ctx, task, credential)
	case "getUser":
		return b.RunGetUser(ctx, task, credential)
	case "getUserActivities":
		return b.RunGetUserActivities(ctx, task, credential)
	case "getUserAssociationsCount":
		return b.RunGetUserAssociationsCount(ctx, task, credential)
	case "getUserMemberships":
		return b.RunGetUserMemberships(ctx, task, credential)
	case "getUserStatus":
		return b.RunGetUserStatus(ctx, task, credential)
	case "listEmails":
		return b.RunListEmails(ctx, task, credential)
	case "listEmailsForUser":
		return b.RunListEmailsForUser(ctx, task, credential)
	case "listGPGKeys":
		return b.RunListGPGKeys(ctx, task, credential)
	case "listGPGKeysForUser":
		return b.RunListGPGKeysForUser(ctx, task, credential)
	case "listSSHKeys":
		return b.RunListSSHKeys(ctx, task, credential)
	case "listSSHKeysForUser":
		return b.RunListSSHKeysForUser(ctx, task, credential)
	case "listServiceAccounts":
		return b.RunListServiceAccounts(ctx, task, credential)
	case "listUsers":
		return b.RunListUsers(ctx, task, credential)
	case "modifyUser":
		return b.RunModifyUser(ctx, task, credential)
	case "rejectUser":
		return b.RunRejectUser(ctx, task, credential)
	case "revokeImpersonationToken":
		return b.RunRevokeImpersonationToken(ctx, task, credential)
	case "setUserStatus":
		return b.RunSetUserStatus(ctx, task, credential)
	case "unbanUser":
		return b.RunUnbanUser(ctx, task, credential)
	case "unblockUser":
		return b.RunUnblockUser(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabUsersBundle) GetAction(actionName string) types.Action {
	return h
}
