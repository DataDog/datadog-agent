// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package shell

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ShellBundle provides the runShell action for safe shell execution.
type ShellBundle struct {
	actions map[string]types.Action
}

// NewShellBundle creates a new ShellBundle.
func NewShellBundle() *ShellBundle {
	return &ShellBundle{
		actions: map[string]types.Action{
			"runShell": NewRunShellHandler(),
		},
	}
}

// GetAction returns the action handler for the given name.
func (h *ShellBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
