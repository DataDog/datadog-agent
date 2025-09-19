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
