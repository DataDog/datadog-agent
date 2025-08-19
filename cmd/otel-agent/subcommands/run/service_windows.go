//go:build windows && otlp

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
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
		params := &cliParams{
			GlobalParams: &subcommands.GlobalParams{
				ConfPaths:  []string{"file:C:/Program Files/Datadog/Datadog Agent/bin/agent/dist/otel-config.yaml"},
				ConfigName: "datadog-otel",
				LoggerName: loggerName,
				BYOC:       false,
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
