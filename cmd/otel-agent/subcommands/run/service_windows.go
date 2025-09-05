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
)

const (
	// loggerName is the application logger identifier for service mode
	loggerName = "OTELCOL"
)

// StartOTelAgentWithDefaults starts the otel-agent with default parameters for Windows service
func StartOTelAgentWithDefaults(ctxChan <-chan context.Context) (<-chan error, error) {
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)

		// Wait for context from service
		ctx := <-ctxChan

		// Create default parameters for service mode
		// Prefer ProgramData\Datadog\otel-config.yaml to align with runtime config location
		pd, _ := winutil.GetProgramDataDir()
		// Normalize to avoid duplicate 'Datadog' and handle trailing separators
		ddRoot := strings.TrimRight(pd, "\\/")
		if !strings.EqualFold(filepath.Base(ddRoot), "Datadog") {
			ddRoot = filepath.Join(ddRoot, "Datadog")
		}

		svcCfg := filepath.Join(ddRoot, "otel-config.yaml")
		confURL := "file:" + filepath.ToSlash(svcCfg)

		// Also point core config to ProgramData so otelcollector.enabled is read correctly.
		// NOTE: CoreConfPath expects a native filesystem path, not a confmap URI.
		coreCfgPath := filepath.Join(ddRoot, "datadog.yaml")

		params := &cliParams{
			GlobalParams: &subcommands.GlobalParams{
				ConfPaths:    []string{confURL},
				CoreConfPath: coreCfgPath,
				ConfigName:   "datadog-otel",
				LoggerName:   loggerName,
				BYOC:         false,
			},
			pidfilePath: "", // No pidfile for service mode
		}

		// Run the otel-agent command with the service context
		err := runOTelAgentCommand(ctx, params)
		if err != nil {
			errChan <- err
		}
	}()

	return errChan, nil
}
