// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package run is the run host-profiler subcommand
package run

import (
	"context"

	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	hostprofiler "github.com/DataDog/datadog-agent/comp/host-profiler"
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*globalparams.GlobalParams
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
	return []*cobra.Command{cmd}
}

func runHostProfilerCommand(_ context.Context, cliParams *cliParams) error {
	var opts []fx.Option = []fx.Option{
		hostprofiler.Bundle(collectorimpl.NewParams(cliParams.GlobalParams.ConfFilePath)),
		fx.Invoke(func(collector collector.Component) error {
			return collector.Run()
		}),
	}

	if cliParams.GlobalParams.CoreConfPath != "" {
		opts = append(opts, fx.Provide(collectorimpl.NewExtraFactoriesWithAgentCore))
	} else {
		opts = append(opts, fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore))
	}

	return fxutil.Run(opts...)
}
