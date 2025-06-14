// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestFxRun_WithDatadogExporter(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "true")
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		params := &subcommands.GlobalParams{
			ConfPaths: []string{"test_config.yaml"},
		}
		return runOTelAgentCommand(ctx, params)
	})
}

func TestFxRun_NoDatadogExporter(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "true")
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		params := &subcommands.GlobalParams{
			ConfPaths: []string{"test_config_no_dd.yaml"},
		}
		return runOTelAgentCommand(ctx, params)
	})
}

func TestFxRun_Disabled(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "false")
	ctx := context.Background()
	params := &subcommands.GlobalParams{
		ConfPaths: []string{"test_config.yaml"},
	}
	assert.NoError(t, runOTelAgentCommand(ctx, params))
}
