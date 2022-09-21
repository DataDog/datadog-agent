// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package version implements 'agent version'.
package version

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Agent %s %s- Commit: %s - Serialization version: %s - Go version: %s",
					color.CyanString(av.GetNumberAndPre()),
					meta,
					color.GreenString(av.Commit),
					color.YellowString(serializer.AgentPayloadVersion),
					color.RedString(runtime.Version()),
				),
			)
		},
	}

	return []*cobra.Command{versionCmd}
}
