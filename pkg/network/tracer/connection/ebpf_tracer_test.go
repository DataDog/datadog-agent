// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

// Package connection provides tracing for connections
package connection

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil)
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	_, err = maps.GetMap[int32, uint64](tr.(*ebpfTracer).m.Manager, probes.TCPFailureTelemetry)
	require.NoError(t, err, "error loading tcp failure telemetry map")
}
