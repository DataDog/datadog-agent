// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build otlp

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestInitializeProcessIdentity(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	originalLogsIdentity := metrics.GetAgentIdentityTag()
	t.Cleanup(func() {
		flavor.SetFlavor(originalFlavor)
		metrics.SetAgentIdentity(originalLogsIdentity)
	})

	flavor.SetFlavor(flavor.TraceAgent)
	metrics.SetAgentIdentity("trace-agent")

	initializeProcessIdentity()

	require.Equal(t, flavor.OTelAgent, flavor.GetFlavor())
	require.Equal(t, "otel-agent", metrics.GetAgentIdentityTag())
}
