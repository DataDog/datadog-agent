// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
)

func newTestCiliumLBConntracker(t *testing.T) *ciliumLoadBalancerConntracker {
	t.Helper()
	require.NoError(t, rlimit.RemoveMemlock())

	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  12,
		MaxEntries: 16,
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	backends, err := maps.Map[backend4KeyV3, backend4ValueV3](m)
	require.NoError(t, err)
	return &ciliumLoadBalancerConntracker{
		backends:           backends,
		backendIDToBackend: make(map[uint32]backend),
	}
}

func putBackend(t *testing.T, clb *ciliumLoadBalancerConntracker, id uint32, ip [4]byte) {
	t.Helper()
	require.NoError(t, clb.backends.Put(&backend4KeyV3{ID: id}, &backend4ValueV3{Address: ip, Port: htons(8080)}))
}

func TestCiliumConntrackerUpdateBackends(t *testing.T) {
	clb := newTestCiliumLBConntracker(t)

	putBackend(t, clb, 1, [4]byte{10, 0, 0, 1})
	putBackend(t, clb, 2, [4]byte{10, 0, 0, 2})
	clb.updateBackends()
	require.Len(t, clb.backendIDToBackend, 2)

	// backend 1 is removed, backend 2 changes address and backend 3 is added
	require.NoError(t, clb.backends.Delete(&backend4KeyV3{ID: 1}))
	putBackend(t, clb, 2, [4]byte{10, 0, 0, 22})
	putBackend(t, clb, 3, [4]byte{10, 0, 0, 3})
	clb.updateBackends()

	require.Len(t, clb.backendIDToBackend, 2)
	require.NotContains(t, clb.backendIDToBackend, uint32(1))
	require.Equal(t, "10.0.0.22", clb.backendIDToBackend[2].addr.String())
	require.Equal(t, "10.0.0.3", clb.backendIDToBackend[3].addr.String())
	require.Equal(t, uint16(8080), clb.backendIDToBackend[3].port)
}

func TestCiliumConntrackerUpdateBackendsIterateError(t *testing.T) {
	clb := newTestCiliumLBConntracker(t)

	putBackend(t, clb, 1, [4]byte{10, 0, 0, 1})
	putBackend(t, clb, 2, [4]byte{10, 0, 0, 2})
	clb.updateBackends()
	require.Len(t, clb.backendIDToBackend, 2)

	// iterating a closed map fails, which must not wipe the backends from the previous update
	require.NoError(t, clb.backends.Map().Close())
	clb.updateBackends()

	require.Len(t, clb.backendIDToBackend, 2)
}
