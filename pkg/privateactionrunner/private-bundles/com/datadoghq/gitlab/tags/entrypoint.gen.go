package com_datadoghq_gitlab_tags

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabTagsBundle struct {
	actions map[string]types.Action
}

func NewGitlabTags() types.Bundle {
	return &GitlabTagsBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"createTag": NewCreateTagHandler(),
			"deleteTag": NewDeleteTagHandler(),
			"getTag":    NewGetTagHandler(),
			"listTags":  NewListTagsHandler(),
		},
	}
}

func (h *GitlabTagsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
