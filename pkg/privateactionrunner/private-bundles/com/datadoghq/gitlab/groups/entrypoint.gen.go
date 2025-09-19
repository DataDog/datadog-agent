package com_datadoghq_gitlab_groups

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabGroupsBundle struct {
	actions map[string]types.Action
}

func NewGitlabGroups() types.Bundle {
	return &GitlabGroupsBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"createGroup": NewCreateGroupHandler(),
			"deleteGroup": NewDeleteGroupHandler(),
			"getGroup":    NewGetGroupHandler(),
			"listGroups":  NewListGroupsHandler(),
			"updateGroup": NewUpdateGroupHandler(),
		},
	}
}

func (h *GitlabGroupsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
