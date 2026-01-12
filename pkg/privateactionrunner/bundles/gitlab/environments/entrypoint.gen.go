// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
