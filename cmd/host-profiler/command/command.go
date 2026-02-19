// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package command contains the top-level Cobra command for the host profiler.
package command

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/cmd/host-profiler/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

// MakeRootCommand makes the top-level Cobra command for this app.
func MakeRootCommand() *cobra.Command {
	hostProfiler := &cobra.Command{
		Use:   "host-profiler [command]",
		Short: "Collects host profiling data and sends it to Datadog.",
		Long:  `The Datadog Host Profiler leverages eBPF-based profiling to collect low-overhead, system-wide performance data from Linux hosts. Built on the OpenTelemetry eBPF Profiler, it captures CPU, memory, and other resource usage across all processes, enabling deep visibility into application and system behavior. Collected profiles are sent to Datadog.`,
	}

	globalParams := globalparams.GlobalParams{}
	globalParamsGetter := func() *globalparams.GlobalParams {
		return &globalParams
	}
	hostProfiler.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "config", "c", "", "path to host-profiler configuration file")
	hostProfiler.PersistentFlags().StringVarP(&globalParams.CoreConfPath, "core-config", "", "", "Location to the Datadog Agent config file. If this value is not set, infra attribute processor and all features related to the Agent will not be enabled.")
	hostProfiler.PersistentFlags().DurationVar(&globalParams.SyncOnInitTimeout, "sync-on-init-timeout", 30*time.Second, "How long should config sync retry at initialization before failing.")
	hostProfiler.PersistentFlags().DurationVar(&globalParams.SyncTimeout, "sync-to", 3*time.Second, "Timeout for config sync requests.")
	for _, subCommandFactory := range hostProfilerSubcommands() {
		subcommands := subCommandFactory(globalParamsGetter)
		for _, cmd := range subcommands {
			hostProfiler.AddCommand(cmd)
		}
	}

	return hostProfiler
}

type subcommandFactory func(func() *globalparams.GlobalParams) []*cobra.Command

// hostProfilerSubcommands returns SubcommandFactories for the subcommands supported.
func hostProfilerSubcommands() []subcommandFactory {
	return []subcommandFactory{
		run.MakeCommand,
		func(_ func() *globalparams.GlobalParams) []*cobra.Command {
			return []*cobra.Command{version.MakeCommand("Host profiler")}
		},
	}
}
