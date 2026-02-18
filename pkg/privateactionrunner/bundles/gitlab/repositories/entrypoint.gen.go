// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repositories

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabRepositoriesBundle struct {
	actions map[string]types.Action
}

func NewGitlabRepositories() types.Bundle {
	return &GitlabRepositoriesBundle{
		actions: map[string]types.Action{
			// Manual actions
			"contributors":   NewContributorsHandler(),
			"getBlob":        NewGetBlobHandler(),
			"getFileArchive": NewGetFileArchiveHandler(),
			"rawBlobContent": NewRawBlobContentHandler(),
			// Auto-generated actions
			"addChangelog":          NewAddChangelogHandler(),
			"compare":               NewCompareHandler(),
			"generateChangelogData": NewGenerateChangelogDataHandler(),
			"listTree":              NewListTreeHandler(),
			"mergeBase":             NewMergeBaseHandler(),
		},
	}
}

func (h *GitlabRepositoriesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
