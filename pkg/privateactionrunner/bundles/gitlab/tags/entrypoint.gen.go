// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
