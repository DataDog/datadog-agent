// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
)

type checkMap struct {
	name      string
	mtype     ebpf.MapType
	lockCount uint32
}

var specs map[ebpf.MapType]ebpf.MapSpec = map[ebpf.MapType]ebpf.MapSpec{
	ebpf.Hash: ebpf.MapSpec{
		Name:       "test_hash",
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.PerCPUHash: ebpf.MapSpec{
		Name:       "test_percpu_hash",
		Type:       ebpf.PerCPUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
}

func record(fd int) {
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlCollectLocksCmd, uintptr(0))
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

func TestLockRanges(t *testing.T) {
	cpu, err := kernel.PossibleCPUs()
	require.NoError(t, err)

	cases := []checkMap{
		{
			name:      "Hashmap",
			mtype:     ebpf.Hash,
			lockCount: hashMapLockRanges(cpu),
		},
		{
			name:      "Percpu-Hashmap",
			mtype:     ebpf.PerCPUHash,
			lockCount: hashMapLockRanges(cpu),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := NewLockContentionCollector()
			spec := specs[c.mtype]
			m, err := ebpf.NewMap(&spec)
			require.NoError(t, err)

			mInfo, err := m.Info()
			require.NoError(t, err)

			id, _ := mInfo.ID()
			mapNameMapping[uint32(id)] = spec.Name

			l.InitializeCollector(true)

			require.Equal(t, entries(l.objects.MapAddrFd), c.lockCount)

			m.Close()
			l.Close()
		})
	}
}
