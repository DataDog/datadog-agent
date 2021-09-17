// +build linux_bpf

package kprobe

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/ebpf"
)

const defaultExpiredStateInterval = 60 * time.Second

// perfBatchManager is reponsbile for two things:
//
// * Keeping track of the state of each batch object we read off the perf ring;
// * Detecting idle batches (this might happen in hosts with a low connection churn);
//
// The motivation is to impose an upper limit on how long a TCP close connection
// event remains stored in the eBPF map before being processed by the NetworkAgent.
type perfBatchManager struct {
	// eBPF
	batchMap *ebpf.Map

	// stateByCPU contains the state of each batch.
	// The slice is indexed by the CPU core number.
	stateByCPU []percpuState

	expiredStateInterval time.Duration
}

// newPerfBatchManager returns a new `perfBatchManager` and initializes the
// eBPF map that holds the tcp_close batch objects.
func newPerfBatchManager(batchMap *ebpf.Map, numCPUs int) (*perfBatchManager, error) {
	if batchMap == nil {
		return nil, fmt.Errorf("batchMap is nil")
	}

	state := make([]percpuState, numCPUs)
	for cpu := 0; cpu < numCPUs; cpu++ {
		b := new(netebpf.Batch)
		if err := batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(b)); err != nil {
			return nil, fmt.Errorf("error initializing perf batch manager maps: %w", err)
		}
		state[cpu] = percpuState{
			processed: make(map[uint64]batchState),
		}
	}

	return &perfBatchManager{
		batchMap:             batchMap,
		stateByCPU:           state,
		expiredStateInterval: defaultExpiredStateInterval,
	}, nil
}

// Extract from the given batch all connections that haven't been processed yet.
func (p *perfBatchManager) Extract(b *netebpf.Batch, cpu int) []network.ConnectionStats {
	if cpu >= len(p.stateByCPU) {
		return nil
	}

	batchId := b.Id
	cpuState := &p.stateByCPU[cpu]
	start := uint16(0)
	if bState, ok := cpuState.processed[batchId]; ok {
		start = bState.offset
	}

	buffer := make([]network.ConnectionStats, 0, netebpf.BatchSize-start)
	conns := p.extractBatchInto(buffer, b, start, netebpf.BatchSize)
	delete(cpuState.processed, batchId)

	return conns
}

// GetPendingConns return all connections that are in batches that are not yet full.
// It tracks which connections have been processed by this call, by batch id.
// This prevents double-processing of connections between GetPendingConns and Extract.
func (p *perfBatchManager) GetPendingConns() []network.ConnectionStats {
	var idle []network.ConnectionStats
	b := new(netebpf.Batch)
	for cpu := 0; cpu < len(p.stateByCPU); cpu++ {
		cpuState := &p.stateByCPU[cpu]

		// we have an idle batch, so let's retrieve its data from eBPF
		err := p.batchMap.Lookup(unsafe.Pointer(&cpu), unsafe.Pointer(b))
		if err != nil {
			continue
		}

		batchLen := b.Len
		if batchLen == 0 {
			continue
		}
		// have we already processed these messages?
		start := uint16(0)
		batchId := b.Id
		if bState, ok := cpuState.processed[batchId]; ok {
			start = bState.offset
		}

		idle = p.extractBatchInto(idle, b, start, batchLen)
		// update timestamp regardless since this partial batch still exists
		cpuState.processed[batchId] = batchState{offset: batchLen, updated: time.Now()}
	}

	p.cleanupExpiredState(time.Now())
	return idle
}

type percpuState struct {
	// map of batch id -> offset of conns already processed by GetPendingConns
	processed map[uint64]batchState
}

type batchState struct {
	offset  uint16
	updated time.Time
}

// ExtractBatchInto extract network.ConnectionStats objects from the given `batch` into the supplied `buffer`.
// The `start` (inclusive) and `end` (exclusive) arguments represent the offsets of the connections we're interested in.
func (p *perfBatchManager) extractBatchInto(buffer []network.ConnectionStats, b *netebpf.Batch, start, end uint16) []network.ConnectionStats {
	if start >= end || end > netebpf.BatchSize {
		return buffer
	}

	for i := start; i < end; i++ {
		var ct netebpf.Conn
		switch i {
		case 0:
			ct = b.C0
			break
		case 1:
			ct = b.C1
			break
		case 2:
			ct = b.C2
			break
		case 3:
			ct = b.C3
			break
		default:
			panic("batch size is out of sync")
		}

		conn := connStats(&ct.Tup, &ct.Conn_stats)
		updateTCPStats(&conn, &ct.Tcp_stats)
		buffer = append(buffer, conn)
	}
	return buffer
}

func (p *perfBatchManager) cleanupExpiredState(now time.Time) {
	for cpu := 0; cpu < len(p.stateByCPU); cpu++ {
		cpuState := &p.stateByCPU[cpu]
		for id, s := range cpuState.processed {
			if now.Sub(s.updated) >= p.expiredStateInterval {
				delete(cpuState.processed, id)
			}
		}
	}
}
