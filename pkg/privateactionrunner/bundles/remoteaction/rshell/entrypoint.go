// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

// RshellBundle implements types.Bundle for the com.datadoghq.remoteaction.rshell bundle.
type RshellBundle struct {
	actions map[string]types.Action
}

// NewRshellBundle creates the rshell bundle with its registered actions.
func NewRshellBundle(allowedPaths []string) types.Bundle {
	return &RshellBundle{
		actions: map[string]types.Action{
			"runCommand": NewRunCommandHandler(allowedPaths),
		},
	}
}

// GetAction returns the action registered under actionName, or nil if not found.
func (b *RshellBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
