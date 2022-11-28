// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func VersionCommands(globalParams *common.GlobalParams) []*cobra.Command {
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

			fmt.Fprintf(color.Output, "Security agent %s %s- Commit: '%s' - Serialization version: %s\n",
				color.BlueString(av.GetNumberAndPre()),
				meta,
				color.GreenString(version.Commit),
				color.MagentaString(serializer.AgentPayloadVersion),
			)
		},
	}

	return []*cobra.Command{versionCmd}
}
