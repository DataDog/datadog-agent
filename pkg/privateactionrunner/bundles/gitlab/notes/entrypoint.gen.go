// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabNotesBundle struct{}

func NewGitlabNotes() types.Bundle {
	return &GitlabNotesBundle{}
}

func (b *GitlabNotesBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabNotesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createDraftNote":
		return b.RunCreateDraftNote(ctx, task, credential)
	case "createEpicNote":
		return b.RunCreateEpicNote(ctx, task, credential)
	case "createIssueNote":
		return b.RunCreateIssueNote(ctx, task, credential)
	case "createMergeRequestNote":
		return b.RunCreateMergeRequestNote(ctx, task, credential)
	case "createSnippetNote":
		return b.RunCreateSnippetNote(ctx, task, credential)
	case "deleteDraftNote":
		return b.RunDeleteDraftNote(ctx, task, credential)
	case "deleteEpicNote":
		return b.RunDeleteEpicNote(ctx, task, credential)
	case "deleteIssueNote":
		return b.RunDeleteIssueNote(ctx, task, credential)
	case "deleteMergeRequestNote":
		return b.RunDeleteMergeRequestNote(ctx, task, credential)
	case "deleteSnippetNote":
		return b.RunDeleteSnippetNote(ctx, task, credential)
	case "getDraftNote":
		return b.RunGetDraftNote(ctx, task, credential)
	case "getEpicNote":
		return b.RunGetEpicNote(ctx, task, credential)
	case "getIssueNote":
		return b.RunGetIssueNote(ctx, task, credential)
	case "getMergeRequestNote":
		return b.RunGetMergeRequestNote(ctx, task, credential)
	case "getSnippetNote":
		return b.RunGetSnippetNote(ctx, task, credential)
	case "listDraftNotes":
		return b.RunListDraftNotes(ctx, task, credential)
	case "listEpicNotes":
		return b.RunListEpicNotes(ctx, task, credential)
	case "listIssueNotes":
		return b.RunListIssueNotes(ctx, task, credential)
	case "listMergeRequestNotes":
		return b.RunListMergeRequestNotes(ctx, task, credential)
	case "listSnippetNotes":
		return b.RunListSnippetNotes(ctx, task, credential)
	case "publishAllDraftNotes":
		return b.RunPublishAllDraftNotes(ctx, task, credential)
	case "publishDraftNote":
		return b.RunPublishDraftNote(ctx, task, credential)
	case "updateDraftNote":
		return b.RunUpdateDraftNote(ctx, task, credential)
	case "updateEpicNote":
		return b.RunUpdateEpicNote(ctx, task, credential)
	case "updateIssueNote":
		return b.RunUpdateIssueNote(ctx, task, credential)
	case "updateMergeRequestNote":
		return b.RunUpdateMergeRequestNote(ctx, task, credential)
	case "updateSnippetNote":
		return b.RunUpdateSnippetNote(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
