// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
