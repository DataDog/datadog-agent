package com_datadoghq_gitlab_members

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabMembersBundle struct {
	actions map[string]types.Action
}

func NewGitlabMembers() types.Bundle {
	return &GitlabMembersBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"listProjectMembers": NewListProjectMembersHandler(),
		},
	}
}

func (h *GitlabMembersBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
