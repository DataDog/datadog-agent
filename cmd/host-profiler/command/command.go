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
	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/cmd/host-profiler/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

func normalizeCoreConfig(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "core-config":
		name = "agent-config"
	}
	return pflag.NormalizedName(name)
}

// MakeRootCommand makes the top-level Cobra command for this app.
// The root command is the `run` command, so `host-profiler` and `host-profiler run` are equivalent.
func MakeRootCommand() *cobra.Command {
	globalParams := globalparams.GlobalParams{}
	globalParamsGetter := func() *globalparams.GlobalParams {
		return &globalParams
	}

	runCmds := run.MakeCommand(globalParamsGetter)
	hostProfiler := *runCmds[0]
	hostProfiler.Use = "host-profiler [command]"
	hostProfiler.Short = "Collects host profiling data and sends it to Datadog."
	hostProfiler.Long = `The Datadog Host Profiler leverages eBPF-based profiling to collect low-overhead, system-wide performance data from Linux hosts. Built on the OpenTelemetry eBPF Profiler, it captures CPU, memory, and other resource usage across all processes, enabling deep visibility into application and system behavior. Collected profiles are sent to Datadog.`

	hostProfiler.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "config", "c", "", "path to host-profiler configuration file")
	hostProfiler.PersistentFlags().StringVarP(&globalParams.CoreConfPath, "core-config", "", "", "Location to the Datadog Agent config file. If this value is not set, infra attribute processor and all features related to the Agent will not be enabled.")
	hostProfiler.PersistentFlags().DurationVar(&globalParams.SyncOnInitTimeout, "sync-on-init-timeout", 30*time.Second, "How long should config sync retry at initialization before failing.")
	hostProfiler.PersistentFlags().DurationVar(&globalParams.SyncTimeout, "sync-to", 3*time.Second, "Timeout for config sync requests.")
	// Add --agent-config as an alias for --core-config
	hostProfiler.PersistentFlags().StringVar(&globalParams.CoreConfPath, "agent-config", "", "alias for --core-config")

	// hostProfiler is a shallow copy of runCmds[0] so normalizeCoreConfig also applies to runCmds[0] subcommand below.
	hostProfiler.Flags().SetNormalizeFunc(normalizeCoreConfig)
	hostProfiler.AddCommand(runCmds[0])
	hostProfiler.AddCommand(version.MakeCommand("Host profiler"))

	return &hostProfiler
}
