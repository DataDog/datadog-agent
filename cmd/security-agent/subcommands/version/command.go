// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package version holds version related files
package version

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

// Commands returns the global params commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(displayVersion,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    logimpl.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{versionCmd}
}

func displayVersion(_ log.Component, _ config.Component, _ secrets.Component) {
	av, _ := pkgversion.Agent()
	meta := ""
	if av.Meta != "" {
		meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
	}

	fmt.Fprintf(color.Output, "Security agent %s %s- Commit: '%s' - Serialization version: %s\n",
		color.BlueString(av.GetNumberAndPre()),
		meta,
		color.GreenString(pkgversion.Commit),
		color.MagentaString(serializer.AgentPayloadVersion),
	)
}
