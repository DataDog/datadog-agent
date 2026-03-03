// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sandboxedshell

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// SandboxedShellBundle provides shell execution inside an agentfs overlay
// filesystem sandbox, combining AST-based script verification with filesystem
// isolation.
type SandboxedShellBundle struct {
	actions map[string]types.Action
}

// NewSandboxedShellBundle creates a new SandboxedShellBundle.
func NewSandboxedShellBundle() *SandboxedShellBundle {
	return &SandboxedShellBundle{
		actions: map[string]types.Action{
			"runSandboxed": NewRunSandboxedHandler(),
			"closeSession": NewCloseSessionHandler(),
			"getManual":    NewGetManualHandler(),
		},
	}
}

// GetAction returns the action handler for the given name.
func (h *SandboxedShellBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
