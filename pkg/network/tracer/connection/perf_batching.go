// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"fmt"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

// perfBatchManager is responsible for two things:
//
// * Keeping track of the state of each batch object we read off the perf ring;
// * Detecting idle batches (this might happen in hosts with a low connection churn);
//
// The motivation is to impose an upper limit on how long a TCP close connection
// event remains stored in the eBPF map before being processed by the NetworkAgent.
type perfBatchManager struct {
	batchMap  *maps.GenericMap[uint32, netebpf.Batch]
	extractor *batchExtractor
	ch        *cookieHasher
}

// newPerfBatchManager returns a new `PerfBatchManager` and initializes the
// eBPF map that holds the tcp_close batch objects.
func newPerfBatchManager(batchMap *maps.GenericMap[uint32, netebpf.Batch], extractor *batchExtractor) (*perfBatchManager, error) {
	if batchMap == nil {
		return nil, fmt.Errorf("batchMap is nil")
	}

	for cpu := uint32(0); cpu < uint32(extractor.NumCPUs()); cpu++ {
		b := new(netebpf.Batch)
		// Ring buffer events don't have CPU information, so we associate each
		// batch entry with a CPU during startup. This information is used by
		// the code that does the batch offset tracking.
		b.Cpu = cpu
		if err := batchMap.Put(&cpu, b); err != nil {
			return nil, fmt.Errorf("error initializing perf batch manager maps: %w", err)
		}
	}

	return &perfBatchManager{
		batchMap:  batchMap,
		extractor: extractor,
		ch:        newCookieHasher(),
	}, nil
}

// ExtractBatchInto extracts from the given batch all connections that haven't been processed yet.
func (p *perfBatchManager) ExtractBatchInto(buffer *network.ConnectionBuffer, b *netebpf.Batch) {
	for rc := p.extractor.NextConnection(b); rc != nil; rc = p.extractor.NextConnection(b) {
		conn := buffer.Next()
		populateConnStats(conn, &rc.Tup, &rc.Conn_stats, p.ch)
		updateTCPStats(conn, &rc.Tcp_stats, rc.Tcp_retransmits)
	}
}

// GetPendingConns return all connections that are in batches that are not yet full.
// It tracks which connections have been processed by this call, by batch id.
// This prevents double-processing of connections between GetPendingConns and Extract.
func (p *perfBatchManager) GetPendingConns(buffer *network.ConnectionBuffer) {
	b := new(netebpf.Batch)
	for cpu := uint32(0); cpu < uint32(p.extractor.NumCPUs()); cpu++ {
		err := p.batchMap.Lookup(&cpu, b)
		if err != nil {
			continue
		}

		for rc := p.extractor.NextConnection(b); rc != nil; rc = p.extractor.NextConnection(b) {
			c := buffer.Next()
			populateConnStats(c, &rc.Tup, &rc.Conn_stats, p.ch)
			updateTCPStats(c, &rc.Tcp_stats, rc.Tcp_retransmits)
		}
	}
	p.extractor.CleanupExpiredState(time.Now())
}

func newConnBatchManager(mgr *manager.Manager, extractor *batchExtractor) (*perfBatchManager, error) {
	connCloseMap, err := maps.GetMap[uint32, netebpf.Batch](mgr, probes.ConnCloseBatchMap)
	if err != nil {
		return nil, fmt.Errorf("unable to get map %s: %s", probes.ConnCloseBatchMap, err)
	}
	batchMgr, err := newPerfBatchManager(connCloseMap, extractor)
	if err != nil {
		return nil, err
	}

	return batchMgr, nil
}
