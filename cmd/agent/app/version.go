// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version info",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if flagNoColor {
			color.NoColor = true
		}
		av, _ := version.New(version.AgentVersion, version.Commit)
		meta := ""
		if av.Meta != "" {
			meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
		}
		fmt.Fprintln(
			color.Output,
			fmt.Sprintf("Agent %s %s- Commit: %s - Serialization version: %s",
				color.BlueString(av.GetNumberAndPre()),
				meta,
				color.GreenString(av.Commit),
				color.MagentaString(serializer.AgentPayloadVersion),
			),
		)
	},
}
