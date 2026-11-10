// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package com_datadoghq_remoteaction_rshell

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// RshellBundle implements types.Bundle for the com.datadoghq.remoteaction.rshell bundle.
type RshellBundle struct {
	actions map[string]types.Action
}

// NewRshellBundle creates the rshell bundle with its registered actions.
// It reads the operator-configured path, command, and system-service allowlists
// from the config.
func NewRshellBundle(cfg config.Config) types.Bundle {
	return &RshellBundle{
		actions: map[string]types.Action{
			"runCommand":            &RunCommandHandler{config: cfg, mode: interp.ModeReadOnly},
			"runRemediationCommand": &RunCommandHandler{config: cfg, mode: interp.ModeRemediation},
		},
	}
}

// GetAction returns the action registered under actionName, or nil if not found.
func (b *RshellBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}

func commandConfig(cfg config.Config) commandPolicy {
	paths, pathsConfigured := cfg.RestrictedShellAllowedPaths()
	commands, commandsConfigured := cfg.RestrictedShellAllowedCommands()
	return commandPolicy{
		OperatorAllowedPaths:              paths,
		OperatorAllowedCommands:           commands,
		OperatorAllowedPathsConfigured:    pathsConfigured,
		OperatorAllowedCommandsConfigured: commandsConfigured,
		OperatorAllowedSystemServices:     cfg.RestrictedShellAllowedSystemServices(),
		OperatorElevatableCommands:        cfg.RestrictedShellElevatableCommands(),
		DisableDetailedTelemetry:          cfg.RestrictedShellDisableDetailedTelemetry(),
		PrivilegedEnabled:                 cfg.RestrictedShellPrivilegedEnabled(),
		PrivilegedSocket:                  cfg.RestrictedShellPrivilegedSocket(),
	}
}
