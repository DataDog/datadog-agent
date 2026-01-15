// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_commits

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabCommitsBundle struct {
	actions map[string]types.Action
}

func NewGitlabCommits() types.Bundle {
	return &GitlabCommitsBundle{
		actions: map[string]types.Action{
			// Manual actions
			"revertCommit": NewRevertCommitHandler(),
			// Auto-generated actions
			"cherryPickCommit":          NewCherryPickCommitHandler(),
			"createCommit":              NewCreateCommitHandler(),
			"getCommit":                 NewGetCommitHandler(),
			"getCommitComments":         NewGetCommitCommentsHandler(),
			"getCommitDiff":             NewGetCommitDiffHandler(),
			"getCommitRefs":             NewGetCommitRefsHandler(),
			"getCommitStatuses":         NewGetCommitStatusesHandler(),
			"getGPGSignature":           NewGetGPGSignatureHandler(),
			"listCommits":               NewListCommitsHandler(),
			"listMergeRequestsByCommit": NewListMergeRequestsByCommitHandler(),
			"postCommitComment":         NewPostCommitCommentHandler(),
			"setCommitStatus":           NewSetCommitStatusHandler(),
		},
	}
}

func (h *GitlabCommitsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
