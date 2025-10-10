// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp

package run

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
	"github.com/spf13/cobra"
)

// TryToGetDefaultParamsIfMissing fills missing config paths with ProgramData defaults on Windows.
// It does not override values already provided via flags or environment.
func TryToGetDefaultParamsIfMissing(p *cliParams) {
	// Fallbacks if a path is missing/not provided
	if len(p.ConfPaths) == 0 || p.CoreConfPath == "" {
		pd, _ := winutil.GetProgramDataDir()
		root := strings.TrimRight(pd, "\\/")
		if !strings.EqualFold(filepath.Base(root), "Datadog") {
			root = filepath.Join(root, "Datadog")
		}
		if len(p.ConfPaths) == 0 {
			cfg := filepath.Join(root, "otel-config.yaml")
			p.ConfPaths = []string{"file:" + filepath.ToSlash(cfg)}
		}
		if p.CoreConfPath == "" {
			p.CoreConfPath = filepath.Join(root, "datadog.yaml")
		}
	}
}

// MakeCommand creates the 'run' command on Windows
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(_ *cobra.Command, _ []string) error {
			params.GlobalParams = globalConfGetter()
			if servicemain.RunningAsWindowsService() {
				servicemain.Run(&service{cliParams: params})
				return nil
			}
			TryToGetDefaultParamsIfMissing(params)
			return runOTelAgentCommand(context.Background(), params)
		},
	}
	return cmd
}
