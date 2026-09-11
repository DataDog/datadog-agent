// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestDarwinPacketEnrichmentStatusTransitions(t *testing.T) {
	require.Equal(t, darwinSidecarDisabled, darwinPacketEnrichmentStatus(false, false, nil, darwinPacketSidecarStats{}))
	require.Equal(t, darwinSidecarDisabled, darwinPacketEnrichmentStatus(true, false, errors.New("no iface"), darwinPacketSidecarStats{}))
	require.Equal(t, darwinSidecarStopped, darwinPacketEnrichmentStatus(true, true, errors.New("read failed"), darwinPacketSidecarStats{}))
	require.Equal(t, darwinSidecarStopped, darwinPacketEnrichmentStatus(true, true, nil, darwinPacketSidecarStats{stopped: true}))
	require.Equal(t, darwinSidecarHealthy, darwinPacketEnrichmentStatus(true, true, nil, darwinPacketSidecarStats{packets: 4, unmatched: 1}))
	require.Equal(t, darwinSidecarDegraded, darwinPacketEnrichmentStatus(true, true, nil, darwinPacketSidecarStats{
		packets:   20,
		unmatched: 12,
	}))
}

func TestDarwinCompositeReportsDisabledPacketEnrichmentWithoutSidecar(t *testing.T) {
	primary := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	composite := newDarwinCompositeTracerWithComponents(primary, nil, nil)
	composite.packetRequested = false

	status := composite.darwinStatus()
	require.Equal(t, "nstat", status.ActiveBackend)
	require.Equal(t, nstat.ABIRevision, status.ABIRevision)
	require.Equal(t, darwinSidecarDisabled, status.PacketEnrichment)
	require.True(t, status.SourceHealthy)
}

func TestDarwinCompositeReportsStoppedPacketEnrichmentOnSidecarFailure(t *testing.T) {
	primary := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	packet := newDarwinPacketSidecar(&fakeDarwinPacketSource{}, primary, 10)
	composite := newDarwinCompositeTracerWithComponents(primary, packet, nil)
	composite.handlePacketFailure(errors.New("pcap read error"))

	status := GetDarwinTracerStatus(composite)
	require.Equal(t, darwinSidecarStopped, status.PacketEnrichment)
	require.Contains(t, status.LastError, "pcap read error")
	require.True(t, status.SourceHealthy)
}

func TestPacketEnrichmentDegradedIsNotHealthy(t *testing.T) {
	status := darwinPacketEnrichmentStatus(true, true, nil, darwinPacketSidecarStats{
		packets:   40,
		unmatched: 30,
	})
	require.Equal(t, darwinSidecarDegraded, status)
	require.NotEqual(t, darwinSidecarHealthy, status)
}
