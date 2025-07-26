// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package com_datadoghq_script provides script functionality for private action bundles.
package com_datadoghq_script //nolint:revive

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Script provides script-related actions for private action bundles.
type Script struct {
	actions map[string]types.Action
}

// NewScript creates a new Script instance.
func NewScript() *Script {
	return &Script{
		actions: map[string]types.Action{
			"runPredefinedScript": NewRunPredefinedScriptHandler(),
		},
	}
}

// GetAction returns the action with the specified name.
func (h *Script) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
