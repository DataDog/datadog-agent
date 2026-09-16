// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && otlp

package run

import (
	"context"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/spf13/cobra"
)

func fleetPoliciesDirFromPlatform() string {
	return ""
}

// defaultCoreConfPath returns candidate when it is a usable fallback for a
// missing --core-config, and "" otherwise.
//
// Without a core config the Datadog config file is never read, so the IPC cert
// and auth token resolve against the process working directory rather than the
// core Agent's config directory and the handshake fails with "certificate
// signed by unknown authority". Package units and Helm pass --core-config; the
// container entrypoints do not.
//
// The file must exist: LoadDatadog surfaces the ReadInConfig error, so
// defaulting to an absent path would boot-loop deployments that legitimately
// run without a core Agent.
func defaultCoreConfPath(candidate string) string {
	// A standalone collector has no core Agent to share IPC artifacts with, so
	// it can function normally without having to load a core config.
	// Furthermore, the ddot-collector image contains a default core config at the
	// default path, while the users not including `--core-config` in the CLI args
	// do not intend for it to be loaded.
	if standalone, err := strconv.ParseBool(os.Getenv("DD_OTEL_STANDALONE")); err == nil && standalone {
		return ""
	}

	// Stat follows symlinks, which is what the container entrypoints create.
	fi, err := os.Stat(candidate)
	if err != nil || fi.IsDir() {
		return ""
	}
	return candidate
}

// TryToGetDefaultParamsIfMissing fills a missing core config path with the
// default datadog.yaml location. It does not override --core-config or
// DD_CORE_CONFIG.
func TryToGetDefaultParamsIfMissing(p *cliParams) {
	if p.CoreConfPath != "" {
		return
	}
	p.CoreConfPath = defaultCoreConfPath(defaultpaths.GetDefaultConfFile())
}

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalConfGetter()
			params.GlobalParams = globalParams
			TryToGetDefaultParamsIfMissing(params)
			return runOTelAgentCommand(context.Background(), params)
		},
	}
	cmd.Flags().StringVarP(&params.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return cmd
}
