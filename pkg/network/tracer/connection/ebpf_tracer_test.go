// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

// Package connection provides tracing for connections
package connection

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil, options{})
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	require.NotNil(t, tr.(*ebpfTracer).tcpFailuresTelemetryMap, "error loading tcp failure telemetry map")
}

func TestConntrackNATMapLoadFailureFallsBack(t *testing.T) {
	// a key size the placeholder doesn't have, so loading with it fails
	bad, err := ebpf.NewMap(&ebpf.MapSpec{Type: ebpf.Hash, KeySize: 4, ValueSize: 4, MaxEntries: 1})
	require.NoError(t, err)
	t.Cleanup(func() { bad.Close() })

	cfg := config.New()
	tr, err := newEbpfTracer(cfg, nil, options{conntrackNATMap: bad})
	require.NoError(t, err, "tracer should load without the shared map")
	t.Cleanup(tr.Stop)
	if tr.Type() == TracerTypeSK {
		t.Skip("the sk tracer doesn't use the conntrack map")
	}

	ct, err := tr.GetMap(probes.ConntrackMap)
	require.NoError(t, err)
	ctInfo, err := ct.Info()
	require.NoError(t, err)
	badInfo, err := bad.Info()
	require.NoError(t, err)
	ctID, _ := ctInfo.ID()
	badID, _ := badInfo.ID()
	assert.NotEqual(t, badID, ctID, "tracer kept the bad map")

	// net_dev_queue is attached again, with its map at full size
	skb, err := tr.GetMap(probes.ConnectionTupleToSocketSKBConnMap)
	require.NoError(t, err)
	assert.Equal(t, cfg.MaxTrackedConnections, skb.MaxEntries())
}
