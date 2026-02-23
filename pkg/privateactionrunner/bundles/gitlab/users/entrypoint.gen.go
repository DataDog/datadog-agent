// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabUsersBundle struct {
	actions map[string]types.Action
}

func NewGitlabUsers() types.Bundle {
	return &GitlabUsersBundle{
		actions: map[string]types.Action{
			// Manual actions
			"createServiceAccountUser": NewCreateServiceAccountUserHandler(),
			"testConnection":           NewTestConnectionHandler(),
			// Auto-generated actions
			"activateUser":              NewActivateUserHandler(),
			"addEmail":                  NewAddEmailHandler(),
			"addEmailForUser":           NewAddEmailForUserHandler(),
			"addGPGKey":                 NewAddGPGKeyHandler(),
			"addGPGKeyForUser":          NewAddGPGKeyForUserHandler(),
			"addSSHKey":                 NewAddSSHKeyHandler(),
			"addSSHKeyForUser":          NewAddSSHKeyForUserHandler(),
			"approveUser":               NewApproveUserHandler(),
			"banUser":                   NewBanUserHandler(),
			"blockUser":                 NewBlockUserHandler(),
			"createUser":                NewCreateUserHandler(),
			"createUserRunner":          NewCreateUserRunnerHandler(),
			"currentUser":               NewCurrentUserHandler(),
			"currentUserStatus":         NewCurrentUserStatusHandler(),
			"deactivateUser":            NewDeactivateUserHandler(),
			"deleteEmail":               NewDeleteEmailHandler(),
			"deleteEmailForUser":        NewDeleteEmailForUserHandler(),
			"deleteGPGKey":              NewDeleteGPGKeyHandler(),
			"deleteGPGKeyForUser":       NewDeleteGPGKeyForUserHandler(),
			"deleteSSHKey":              NewDeleteSSHKeyHandler(),
			"deleteSSHKeyForUser":       NewDeleteSSHKeyForUserHandler(),
			"deleteUser":                NewDeleteUserHandler(),
			"disableTwoFactor":          NewDisableTwoFactorHandler(),
			"getAllImpersonationTokens": NewGetAllImpersonationTokensHandler(),
			"getEmail":                  NewGetEmailHandler(),
			"getGPGKey":                 NewGetGPGKeyHandler(),
			"getGPGKeyForUser":          NewGetGPGKeyForUserHandler(),
			"getImpersonationToken":     NewGetImpersonationTokenHandler(),
			"getSSHKey":                 NewGetSSHKeyHandler(),
			"getSSHKeyForUser":          NewGetSSHKeyForUserHandler(),
			"getUser":                   NewGetUserHandler(),
			"getUserActivities":         NewGetUserActivitiesHandler(),
			"getUserAssociationsCount":  NewGetUserAssociationsCountHandler(),
			"getUserMemberships":        NewGetUserMembershipsHandler(),
			"getUserStatus":             NewGetUserStatusHandler(),
			"listEmails":                NewListEmailsHandler(),
			"listEmailsForUser":         NewListEmailsForUserHandler(),
			"listGPGKeys":               NewListGPGKeysHandler(),
			"listGPGKeysForUser":        NewListGPGKeysForUserHandler(),
			"listSSHKeys":               NewListSSHKeysHandler(),
			"listSSHKeysForUser":        NewListSSHKeysForUserHandler(),
			"listServiceAccounts":       NewListServiceAccountsHandler(),
			"listUsers":                 NewListUsersHandler(),
			"modifyUser":                NewModifyUserHandler(),
			"rejectUser":                NewRejectUserHandler(),
			"revokeImpersonationToken":  NewRevokeImpersonationTokenHandler(),
			"setUserStatus":             NewSetUserStatusHandler(),
			"unbanUser":                 NewUnbanUserHandler(),
			"unblockUser":               NewUnblockUserHandler(),
		},
	}
}

func (h *GitlabUsersBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
