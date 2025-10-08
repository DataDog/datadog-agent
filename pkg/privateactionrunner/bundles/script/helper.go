// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package com_datadoghq_script provides script functionality for private action bundles.
package com_datadoghq_script //nolint:revive

import (
	"context"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	// Used in local mode and for unit tests
	log.Warn("LocalMode / TestMode : script actions will be run by the user running the runner, not by scriptuser")
}

// NewPredefinedScriptCommand creates a new command for running predefined scripts.
func NewPredefinedScriptCommand(ctx context.Context, command []string) *exec.Cmd {
	return exec.CommandContext(ctx, command[0], command[1:]...)
}
