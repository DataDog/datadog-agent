// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	pidMax uint32 = 1 << 22 // PID_MAX_LIMIT on 64bit systems
)

func TestGetPendingConns(t *testing.T) {
	cb, cbCh := ddsync.CallbackChannel[*network.ConnectionStats](100)
	manager := newTestBatchManager(t, cb)

	batch := new(netebpf.Batch)
	batch.Id = 0
	batch.C0.Tup.Pid = pidMax + 1
	batch.C1.Tup.Pid = pidMax + 2
	batch.Len = 2

	cpu := uint32(0)
	updateBatch := func() {
		err := manager.batchMap.Put(&cpu, batch)
		require.NoError(t, err)
	}
	updateBatch()

	manager.Flush()
	var pendingConns []*network.ConnectionStats
	for conn := range cbCh {
		if conn == nil {
			break
		}
		pendingConns = append(pendingConns, conn)
	}
	assert.GreaterOrEqual(t, len(pendingConns), 2)
	for _, pid := range []uint32{pidMax + 1, pidMax + 2} {
		found := false
		for p := range pendingConns {
			if pendingConns[p].Pid == pid {
				found = true
				pendingConns = append(pendingConns[:p], pendingConns[p+1:]...)
				break
			}
		}

		assert.True(t, found, "could not find batched connection for pid %d", pid)
	}

	// Now let's pretend a new connection was added to the batch on eBPF side
	batch.C2.Tup.Pid = pidMax + 3
	batch.Len++
	updateBatch()

	// We should now get only the connection that hasn't been processed before
	manager.Flush()
	pendingConns = pendingConns[:0]
	for conn := range cbCh {
		if conn == nil {
			break
		}
		pendingConns = append(pendingConns, conn)
	}
	assert.GreaterOrEqual(t, len(pendingConns), 1)
	var found bool
	for _, p := range pendingConns {
		if p.Pid == pidMax+3 {
			found = true
			break
		}
	}

	assert.True(t, found, "could not find batched connection for pid %d", pidMax+3)
}

func TestPerfBatchStateCleanup(t *testing.T) {
	cb, cbCh := ddsync.CallbackChannel[*network.ConnectionStats](100)
	manager := newTestBatchManager(t, cb)
	manager.extractor.expiredStateInterval = 100 * time.Millisecond

	batch := new(netebpf.Batch)
	batch.Id = 0
	batch.C0.Tup.Pid = 1
	batch.C1.Tup.Pid = 2
	batch.Len = 2

	cpu := uint32(0)
	err := manager.batchMap.Put(&cpu, batch)
	require.NoError(t, err)

	manager.Flush()
	for conn := range cbCh {
		if conn == nil {
			break
		}
	}

	_, ok := manager.extractor.stateByCPU[cpu].processed[batch.Id]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.extractor.stateByCPU[cpu].processed[batch.Id].offset)

	manager.extractor.CleanupExpiredState(time.Now().Add(manager.extractor.expiredStateInterval))
	manager.Flush()
	for conn := range cbCh {
		if conn == nil {
			break
		}
	}

	// state should not have been cleaned up, since no more connections have happened
	_, ok = manager.extractor.stateByCPU[cpu].processed[batch.Id]
	require.True(t, ok)
	assert.Equal(t, uint16(2), manager.extractor.stateByCPU[cpu].processed[batch.Id].offset)
}

func newTestBatchManager(t *testing.T, callback func(*network.ConnectionStats)) *perfBatchManager {
	require.NoError(t, rlimit.RemoveMemlock())
	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  netebpf.SizeofBatch,
		MaxEntries: numTestCPUs,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	gm, err := ebpfmaps.Map[uint32, netebpf.Batch](m)
	require.NoError(t, err)
	extractor := newBatchExtractor(numTestCPUs)
	connPool := ddsync.NewDefaultTypedPool[network.ConnectionStats]()
	mgr, err := newPerfBatchManager(gm, extractor, connPool, callback)
	require.NoError(t, err)
	return mgr
}
