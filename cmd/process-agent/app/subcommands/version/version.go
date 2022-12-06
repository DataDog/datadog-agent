// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package version

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

// Commands returns a slice of subcommands for the 'process-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	versionCmd := version.MakeCommand("Agent")

	return []*cobra.Command{versionCmd}
}
