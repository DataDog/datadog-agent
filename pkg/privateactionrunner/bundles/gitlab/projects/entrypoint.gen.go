// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabProjectsBundle struct{}

func NewGitlabProjects() types.Bundle {
	return &GitlabProjectsBundle{}
}

func (b *GitlabProjectsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	// Manual actions
	case "createProject":
		return b.CreateProject(ctx, task, credential)
	case "createProjectForUser":
		return b.CreateProjectForUser(ctx, task, credential)
	case "editProject":
		return b.EditProject(ctx, task, credential)
	case "importMembers":
		return b.ImportMembers(ctx, task, credential)
	case "listProjectInvitedGroups":
		return b.ListProjectInvitedGroups(ctx, task, credential)
	case "listShareableGroups":
		return b.ListShareableGroups(ctx, task, credential)
	case "listTransferableGroups":
		return b.ListTransferableGroups(ctx, task, credential)
	case "restoreProject":
		return b.RestoreProject(ctx, task, credential)
	// Auto-generated actions
	case "archiveProject":
		return b.ArchiveProject(ctx, task, credential)
	case "deleteProject":
		return b.DeleteProject(ctx, task, credential)
	case "deleteSharedProjectFromGroup":
		return b.DeleteSharedProjectFromGroup(ctx, task, credential)
	case "getProject":
		return b.GetProject(ctx, task, credential)
	case "getProjectLanguages":
		return b.GetProjectLanguages(ctx, task, credential)
	case "listProjects":
		return b.ListProjects(ctx, task, credential)
	case "listProjectsGroups":
		return b.ListProjectsGroups(ctx, task, credential)
	case "listProjectsUsers":
		return b.ListProjectsUsers(ctx, task, credential)
	case "listUserContributedProjects":
		return b.ListUserContributedProjects(ctx, task, credential)
	case "listUserProjects":
		return b.ListUserProjects(ctx, task, credential)
	case "shareProjectWithGroup":
		return b.ShareProjectWithGroup(ctx, task, credential)
	case "starProject":
		return b.StarProject(ctx, task, credential)
	case "startHousekeepingProject":
		return b.StartHousekeepingProject(ctx, task, credential)
	case "transferProject":
		return b.TransferProject(ctx, task, credential)
	case "unarchiveProject":
		return b.UnarchiveProject(ctx, task, credential)
	case "unstarProject":
		return b.UnstarProject(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabProjectsBundle) GetAction(actionName string) types.Action {
	return h
}
