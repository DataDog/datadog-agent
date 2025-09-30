// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package run is the run host-profiler subcommand
package run

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	hostprofiler "github.com/DataDog/datadog-agent/comp/host-profiler"
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*globalparams.GlobalParams
	SyncTimeout       time.Duration
	SyncOnInitTimeout time.Duration
}

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *globalparams.GlobalParams) []*cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start Host Profiler",
		Long:  `Runs the Host Profiler to collect host profiling data and send it to Datadog.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			params.GlobalParams = globalConfGetter()
			return runHostProfilerCommand(context.Background(), params)
		},
	}
	cmd.Flags().DurationVar(&params.SyncTimeout, "sync-timeout", 3*time.Second, "Timeout for config sync requests.")
	cmd.Flags().DurationVar(&params.SyncOnInitTimeout, "sync-on-init-timeout", 0, "How long should config sync retry at initialization before failing.")

	return []*cobra.Command{cmd}
}

func runHostProfilerCommand(_ context.Context, cliParams *cliParams) error {
	var opts []fx.Option = []fx.Option{
		hostprofiler.Bundle(collectorimpl.NewParams(cliParams.GlobalParams.ConfFilePath)),
	}

	if cliParams.GlobalParams.CoreConfPath != "" {
		opts = append(opts,
			core.Bundle(),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(cliParams.GlobalParams.CoreConfPath),
				SecretParams: secrets.NewEnabledParams(),
				LogParams:    log.ForDaemon(command.LoggerName, "log_file", setup.DefaultHostProfilerLogFile),
			}),

			ipcfx.ModuleReadOnly(),
			remoteTaggerFx.Module(tagger.NewRemoteParams()),
			configsyncimpl.Module(configsyncimpl.NewParams(cliParams.SyncTimeout, true, cliParams.SyncOnInitTimeout)),
			fx.Provide(collectorimpl.NewExtraFactoriesWithAgentCore))
	} else {
		opts = append(opts, fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore))
	}

	return fxutil.OneShot(func(collector collector.Component) error {
		return collector.Run()
	}, opts...)
}
