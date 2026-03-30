// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package socketcontention

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"

	ebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestConversionHelpers(t *testing.T) {
	t.Run("object kind", func(t *testing.T) {
		require.Equal(t, "socket", toObjectKind(1))
		require.Equal(t, "unknown", toObjectKind(99))
	})

	t.Run("socket type", func(t *testing.T) {
		require.Equal(t, "stream", toSocketType(1))
		require.Equal(t, "dgram", toSocketType(2))
		require.Equal(t, "raw", toSocketType(3))
		require.Equal(t, "seqpacket", toSocketType(5))
		require.Equal(t, "unknown", toSocketType(99))
	})

	t.Run("family", func(t *testing.T) {
		require.Equal(t, "unix", toFamily(1))
		require.Equal(t, "inet", toFamily(2))
		require.Equal(t, "inet6", toFamily(10))
		require.Equal(t, "unknown", toFamily(99))
	})

	t.Run("protocol", func(t *testing.T) {
		require.Equal(t, "tcp", toProtocol(6))
		require.Equal(t, "udp", toProtocol(17))
		require.Equal(t, "unknown", toProtocol(0))
		require.Equal(t, "255", toProtocol(255))
	})

	t.Run("lock subtype", func(t *testing.T) {
		require.Equal(t, "sk_lock", toLockSubtype(1))
		require.Equal(t, "sk_wait_queue", toLockSubtype(2))
		require.Equal(t, "callback_lock", toLockSubtype(3))
		require.Equal(t, "error_queue_lock", toLockSubtype(4))
		require.Equal(t, "receive_queue_lock", toLockSubtype(5))
		require.Equal(t, "write_queue_lock", toLockSubtype(6))
		require.Equal(t, "unknown", toLockSubtype(99))
	})
}

func TestGetAndFlushAggregatesPerCPUStats(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	statsMap, err := ebpfmaps.NewGenericMap[ebpfSocketContentionKey, []ebpfSocketContentionStats](&ebpf.MapSpec{
		Name:       statsMapName,
		Type:       ebpf.PerCPUHash,
		MaxEntries: 8,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = statsMap.Map().Close() })

	nbCPU, err := kernel.PossibleCPUs()
	require.NoError(t, err)

	aggregatedKey := ebpfSocketContentionKey{
		Cgroup_id:   42,
		Flags:       3,
		Family:      2,
		Protocol:    6,
		Socket_type: 1,
		Object_kind: 1,
		Lock_subtype: 1,
	}
	aggregatedValues := make([]ebpfSocketContentionStats, nbCPU)
	aggregatedValues[0] = ebpfSocketContentionStats{
		Total_time_ns: 10,
		Min_time_ns:   0,
		Max_time_ns:   7,
		Count:         1,
	}
	aggregatedValues[1] = ebpfSocketContentionStats{
		Total_time_ns: 20,
		Min_time_ns:   5,
		Max_time_ns:   11,
		Count:         2,
	}
	aggregatedValues[2] = ebpfSocketContentionStats{
		Total_time_ns: 30,
		Min_time_ns:   3,
		Max_time_ns:   13,
		Count:         4,
	}
	require.NoError(t, statsMap.Put(&aggregatedKey, &aggregatedValues))

	zeroCountKey := ebpfSocketContentionKey{
		Flags: 7,
	}
	zeroCountValues := make([]ebpfSocketContentionStats, nbCPU)
	require.NoError(t, statsMap.Put(&zeroCountKey, &zeroCountValues))

	unknownKey := ebpfSocketContentionKey{
		Flags: 11,
	}
	unknownValues := make([]ebpfSocketContentionStats, nbCPU)
	unknownValues[0] = ebpfSocketContentionStats{
		Total_time_ns: 9,
		Min_time_ns:   9,
		Max_time_ns:   9,
		Count:         1,
	}
	require.NoError(t, statsMap.Put(&unknownKey, &unknownValues))

	probe := &Probe{statsMap: statsMap}
	stats := probe.GetAndFlush()
	require.Len(t, stats, 1)

	entry := stats[0]
	require.Equal(t, "socket", entry.ObjectKind)
	require.Equal(t, "stream", entry.SocketType)
	require.Equal(t, "inet", entry.Family)
	require.Equal(t, "tcp", entry.Protocol)
	require.Equal(t, "sk_lock", entry.LockSubtype)
	require.Equal(t, uint64(42), entry.CgroupID)
	require.Equal(t, uint32(3), entry.Flags)
	require.Equal(t, uint64(60), entry.TotalTimeNS)
	require.Equal(t, uint64(7), entry.Count)
	require.Equal(t, uint64(3), entry.MinTimeNS)
	require.Equal(t, uint64(13), entry.MaxTimeNS)

	require.Empty(t, probe.GetAndFlush())
}

func TestDebugListLockIdentitiesFormatsEntries(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	lockIdentitiesMap, err := ebpfmaps.NewGenericMap[uint64, ebpfSocketLockIdentity](&ebpf.MapSpec{
		Name:       lockIdentitiesMapName,
		Type:       ebpf.Hash,
		MaxEntries: 8,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lockIdentitiesMap.Map().Close() })

	lockAddr := uint64(0x1234)
	identity := ebpfSocketLockIdentity{
		Sock_ptr:      0x4321,
		Socket_cookie: 99,
		Cgroup_id:     123,
		Family:        10,
		Protocol:      17,
		Socket_type:   2,
		Lock_subtype:  5,
	}
	require.NoError(t, lockIdentitiesMap.Put(&lockAddr, &identity))

	probe := &Probe{lockIdentitiesMap: lockIdentitiesMap}
	identities, err := probe.DebugListLockIdentities()
	require.NoError(t, err)
	require.Len(t, identities, 1)

	entry := identities[0]
	require.Equal(t, lockAddr, entry.LockAddr)
	require.Equal(t, uint64(0x4321), entry.SockPtr)
	require.Equal(t, uint64(99), entry.SocketCookie)
	require.Equal(t, uint64(123), entry.CgroupID)
	require.Equal(t, "inet6", entry.Family)
	require.Equal(t, "udp", entry.Protocol)
	require.Equal(t, "dgram", entry.SocketType)
	require.Equal(t, "receive_queue_lock", entry.LockSubtype)
}
