// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix && !linux

package compliance

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// addPlatformSpecificCommands adds platform-specific compliance subcommands (no-op on non-Linux)
func addPlatformSpecificCommands(_ *cobra.Command, _ *command.GlobalParams) {
	// No platform-specific commands on non-Linux Unix systems
}
