package com_datadoghq_gitlab_environments

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabEnvironmentsBundle struct {
	actions map[string]types.Action
}

func NewGitlabEnvironments() types.Bundle {
	return &GitlabEnvironmentsBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"listEnvironments": NewListEnvironmentsHandler(),
		},
	}
}

func (h *GitlabEnvironmentsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
