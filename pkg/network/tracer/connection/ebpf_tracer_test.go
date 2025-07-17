// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package connection provides tracing for connections
package connection

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil)
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	require.NotNil(t, tr.(*ebpfTracer).tcpFailuresTelemetryMap, "error loading tcp failure telemetry map")
}
