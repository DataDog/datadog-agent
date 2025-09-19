package com_datadoghq_gitlab_graphql

import (
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
)

type GitlabGraphqlBundle struct {
	actions map[string]types.Action
}

func NewGitlabGraphql() types.Bundle {
	return &GitlabGraphqlBundle{
		actions: map[string]types.Action{
			"graphql": NewGraphqlHandler(),
		},
	}
}

func (h *GitlabGraphqlBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
