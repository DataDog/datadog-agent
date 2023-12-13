// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package version

import (
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the `version` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	versionCmd := version.MakeCommand("Agent")

	return []*cobra.Command{versionCmd}
}
