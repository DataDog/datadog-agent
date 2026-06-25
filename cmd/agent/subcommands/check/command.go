// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check implements 'agent check'.
package check

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/check"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := check.MakeCommand(func() check.GlobalParams {
		return check.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			ExtraConfFilePaths:   globalParams.ExtraConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
			FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
			ConfigName:           command.ConfigName,
			LoggerName:           command.LoggerName,
		}
	}, fx.Options(
		wmcatalog.GetCatalog(),
		fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component, filterStore workloadfilter.Component) {
			proccontainers.InitSharedContainerProvider(wmeta, tagger, filterStore)
		}),
	))

	return []*cobra.Command{cmd}
}
