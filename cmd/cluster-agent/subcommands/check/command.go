// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package check implements 'cluster-agent check'.
package check

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-clusteragent"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/check"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := check.MakeCommand(func() check.GlobalParams {
		return check.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	}, fx.Options(
		wmcatalog.GetCatalog(),
		// Required by the kubeapiserver collector, but never enabled in the
		// "check" command because autoscaling doesn't run.
		fx.Supply(autoscalinggate.New()),
		// Create the Leader election engine without initializing it, using the
		// telemetry component provided by fx.
		fx.Invoke(func(tm telemetry.Component) {
			if pkgconfigsetup.Datadog().GetBool("leader_election") {
				ctx, _ := pkgcommon.GetMainCtxCancel()
				leaderelection.CreateGlobalLeaderEngine(ctx, tm)
			}
		}),
	))

	return []*cobra.Command{cmd}
}
