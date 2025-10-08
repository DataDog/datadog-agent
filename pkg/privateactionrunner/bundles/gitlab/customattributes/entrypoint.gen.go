// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_custom_attributes

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabCustomAttributesBundle struct {
	actions map[string]types.Action
}

func NewGitlabCustomAttributes() types.Bundle {
	return &GitlabCustomAttributesBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"deleteCustomGroupAttribute":   NewDeleteCustomGroupAttributeHandler(),
			"deleteCustomProjectAttribute": NewDeleteCustomProjectAttributeHandler(),
			"deleteCustomUserAttribute":    NewDeleteCustomUserAttributeHandler(),
			"getCustomGroupAttribute":      NewGetCustomGroupAttributeHandler(),
			"getCustomProjectAttribute":    NewGetCustomProjectAttributeHandler(),
			"getCustomUserAttribute":       NewGetCustomUserAttributeHandler(),
			"listCustomGroupAttributes":    NewListCustomGroupAttributesHandler(),
			"listCustomProjectAttributes":  NewListCustomProjectAttributesHandler(),
			"listCustomUserAttributes":     NewListCustomUserAttributesHandler(),
			"setCustomGroupAttribute":      NewSetCustomGroupAttributeHandler(),
			"setCustomProjectAttribute":    NewSetCustomProjectAttributeHandler(),
			"setCustomUserAttribute":       NewSetCustomUserAttributeHandler(),
		},
	}
}

func (h *GitlabCustomAttributesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
