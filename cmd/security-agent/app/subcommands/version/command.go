// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/comp/core"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

func Commands(globalParams *common.GlobalParams) []*cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(displayVersion,
				fx.Supply(core.CreateBundleParams(
					"",
					core.WithSecurityAgentConfigFilePaths(globalParams.ConfPathArray),
					core.WithConfigLoadSecurityAgent(true),
				).LogForOneShot(common.LoggerName, "off", true)),
				core.Bundle,
			)
		},
	}

	return []*cobra.Command{versionCmd}
}

func displayVersion(log complog.Component, config compconfig.Component) {
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
}
