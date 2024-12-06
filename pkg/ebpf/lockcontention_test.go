// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
)

type checkMap struct {
	name      string
	mtype     ebpf.MapType
	lockCount uint32
	alloc     func(*ebpf.MapSpec) *ebpf.Map
}

var specs map[ebpf.MapType]ebpf.MapSpec = map[ebpf.MapType]ebpf.MapSpec{
	ebpf.Hash: {
		Name:       "test_hash",
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.PerCPUHash: {
		Name:       "test_percpu_hash",
		Type:       ebpf.PerCPUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.LRUHash: {
		Name:       "test_lru",
		Type:       ebpf.LRUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.LRUCPUHash: {
		Name:       "test_pcpu_lru",
		Type:       ebpf.LRUCPUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.RingBuf: {
		Name:       "test_ringbuf",
		Type:       ebpf.RingBuf,
		MaxEntries: 4096,
	},
}

func entries(mp *ebpf.Map) uint32 {
	iter := mp.Iterate()

	var val uint32
	var key LockRange

	var count uint32
	for iter.Next(&key, &val) {
		count++
	}

	return count
}

func lruLockCount(cpu uint32) uint32 {
	return hashMapLockRanges(cpu) + lruMapLockRanges(cpu)
}

func pcpuLruLockCount(cpu uint32) uint32 {
	return hashMapLockRanges(cpu) + pcpuLruMapLockRanges(cpu)
}

func TestLockRanges(t *testing.T) {
	if !lockContentionCollectorSupported() {
		t.Skip("EBPF lock contention collector not supported")
	}

	cpu, err := kernel.PossibleCPUs()
	require.NoError(t, err)

	cases := []checkMap{
		{
			name:      "Hashmap",
			mtype:     ebpf.Hash,
			lockCount: hashMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "Percpu-Hashmap",
			mtype:     ebpf.PerCPUHash,
			lockCount: hashMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "LRUHash",
			mtype:     ebpf.LRUHash,
			lockCount: lruLockCount(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "LRUPcpuHash",
			mtype:     ebpf.LRUCPUHash,
			lockCount: pcpuLruLockCount(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "RingBuf",
			mtype:     ebpf.RingBuf,
			lockCount: ringbufMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := NewLockContentionCollector()
			require.NotNil(t, l)

			spec := specs[c.mtype]
			m := c.alloc(&spec)

			mInfo, err := m.Info()
			require.NoError(t, err)

			id, _ := mInfo.ID()
			mapNameMapping[uint32(id)] = spec.Name

			err = l.Initialize(false)
			require.NoError(t, err)

			require.Equal(t, entries(l.objects.MapAddrFd), c.lockCount)

			m.Close()
			l.Close()
		})
	}
}

func TestLoadWithMaxTrackedRanges(t *testing.T) {
	if !lockContentionCollectorSupported() {
		t.Skip("EBPF lock contention collector not supported")
	}

	l := NewLockContentionCollector()
	require.NotNil(t, l)

	staticRanges = true
	err := l.Initialize(true)
	require.NoError(t, err)

	l.Close()
}
