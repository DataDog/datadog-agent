// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
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
		av, _ := version.New(version.AgentVersion)
		meta := ""
		if av.Meta != "" {
			meta = fmt.Sprintf("- Meta: %s ", av.Meta)
		}
		fmt.Println(fmt.Sprintf("Agent %s %s- Commit: %s - Serialization version: %s", av.GetNumberAndPre(), meta, av.Commit, serializer.AgentPayloadVersion))
	},
}
