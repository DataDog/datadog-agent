// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_shell

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ShellBundle provides the embedded POSIX shell action.
type ShellBundle struct {
	actions map[string]types.Action
}

// NewShellBundle creates a new shell bundle.
func NewShellBundle() *ShellBundle {
	return &ShellBundle{
		actions: map[string]types.Action{
			"runShell": NewRunShellHandler(),
		},
	}
}

// GetAction returns the action for the given name.
func (b *ShellBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
