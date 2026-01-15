// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package version implements 'private-action-runner version'.
package version

import (
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'private-action-runner' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	versionCmd := version.MakeCommand("Private Action Runner")

	return []*cobra.Command{versionCmd}
}
