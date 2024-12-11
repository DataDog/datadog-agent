// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

const (
	numTestCPUs = 4
)

func TestBatchExtract(t *testing.T) {
	t.Run("normal flush", func(t *testing.T) {
		extractor := newBatchExtractor(numTestCPUs)

		batch := new(netebpf.Batch)
		batch.Len = 4
		batch.Id = 0
		batch.Cpu = 0
		batch.C0.Tup.Pid = 1
		batch.C1.Tup.Pid = 2
		batch.C2.Tup.Pid = 3
		batch.C3.Tup.Pid = 4

		var conns []*netebpf.Conn
		for rc := extractor.NextConnection(batch); rc != nil; rc = extractor.NextConnection(batch) {
			conns = append(conns, rc)
		}
		require.Len(t, conns, 4)
		assert.Equal(t, uint32(1), conns[0].Tup.Pid)
		assert.Equal(t, uint32(2), conns[1].Tup.Pid)
		assert.Equal(t, uint32(3), conns[2].Tup.Pid)
		assert.Equal(t, uint32(4), conns[3].Tup.Pid)
	})

	t.Run("partial flush", func(t *testing.T) {
		extractor := newBatchExtractor(numTestCPUs)
		// Simulate a partial flush
		extractor.stateByCPU[0].processed = map[uint64]batchState{
			0: {offset: 3},
		}

		batch := new(netebpf.Batch)
		batch.Len = 4
		batch.Id = 0
		batch.Cpu = 0
		batch.C0.Tup.Pid = 1
		batch.C1.Tup.Pid = 2
		batch.C2.Tup.Pid = 3
		batch.C3.Tup.Pid = 4

		var conns []*netebpf.Conn
		for rc := extractor.NextConnection(batch); rc != nil; rc = extractor.NextConnection(batch) {
			conns = append(conns, rc)
		}
		assert.Len(t, conns, 1)
		assert.Equal(t, uint32(4), conns[0].Tup.Pid)
	})
}
