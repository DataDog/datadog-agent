// +build linux_bpf

package tracer

/*
#include "../ebpf/c/tracer.h"
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"

	"github.com/DataDog/ebpf"
)

const ConnCloseBatchSize = uint16(C.CONN_CLOSED_BATCH_SIZE)
const defaultExpiredStateInterval = 60 * time.Second

// PerfBatchManager is reponsbile for two things:
//
// * Keeping track of the state of each batch object we read off the perf ring;
// * Detecting idle batches (this might happen in hosts with a low connection churn);
//
// The motivation is to impose an upper limit on how long a TCP close connection
// event remains stored in the eBPF map before being processed by the NetworkAgent.
type PerfBatchManager struct {
	// eBPF
	batchMap *ebpf.Map

	// stateByCPU contains the state of each batch.
	// The slice is indexed by the CPU core number.
	stateByCPU []percpuState

	expiredStateInterval time.Duration
}

// NewPerfBatchManager returns a new `PerfBatchManager` and initializes the
// eBPF map that holds the tcp_close batch objects.
func NewPerfBatchManager(batchMap *ebpf.Map, numCPUs int) (*PerfBatchManager, error) {
	if batchMap == nil {
		return nil, fmt.Errorf("batchMap is nil")
	}

	state := make([]percpuState, numCPUs)
	for cpu := 0; cpu < numCPUs; cpu++ {
		b := new(batch)
		batchMap.Put(unsafe.Pointer(&cpu), unsafe.Pointer(b))
		state[cpu] = percpuState{
			processed: make(map[uint64]batchState),
		}
	}

	return &PerfBatchManager{
		batchMap:             batchMap,
		stateByCPU:           state,
		expiredStateInterval: defaultExpiredStateInterval,
	}, nil
}

// Extract from the given batch all connections that haven't been processed yet.
func (p *PerfBatchManager) Extract(b *batch, cpu int) []network.ConnectionStats {
	if cpu >= len(p.stateByCPU) {
		return nil
	}

	batchId := uint64(b.id)
	cpuState := &p.stateByCPU[cpu]
	start := uint16(0)
	if bState, ok := cpuState.processed[batchId]; ok {
		start = bState.offset
	}

	buffer := make([]network.ConnectionStats, 0, ConnCloseBatchSize-start)
	conns := p.extractBatchInto(buffer, b, start, ConnCloseBatchSize)
	delete(cpuState.processed, batchId)

	return conns
}

// GetIdleConns return all connections that are in batches that are not yet full.
// It tracks which connections have been processed by this call, by batch id.
// This prevents double-processing of connections between GetIdleConns and Extract.
func (p *PerfBatchManager) GetIdleConns() []network.ConnectionStats {
	var idle []network.ConnectionStats
	b := new(batch)
	for cpu := 0; cpu < len(p.stateByCPU); cpu++ {
		cpuState := &p.stateByCPU[cpu]

		// we have an idle batch, so let's retrieve its data from eBPF
		err := p.batchMap.Lookup(unsafe.Pointer(&cpu), unsafe.Pointer(b))
		if err != nil {
			continue
		}

		batchLen := uint16(b.len)
		if batchLen == 0 {
			continue
		}
		// have we already processed these messages?
		start := uint16(0)
		batchId := uint64(b.id)
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
	// map of batch id -> offset of conns already processed by GetIdleConns
	processed map[uint64]batchState
}

type batchState struct {
	offset  uint16
	updated time.Time
}

// ExtractBatchInto extract network.ConnectionStats objects from the given `batch` into the supplied `buffer`.
// The `start` (inclusive) and `end` (exclusive) arguments represent the offsets of the connections we're interested in.
func (p *PerfBatchManager) extractBatchInto(buffer []network.ConnectionStats, b *batch, start, end uint16) []network.ConnectionStats {
	if start >= end || end > ConnCloseBatchSize {
		return buffer
	}

	current := uintptr(unsafe.Pointer(b)) + uintptr(start)*C.sizeof_conn_t
	for i := start; i < end; i++ {
		ct := Conn(*(*C.conn_t)(unsafe.Pointer(current)))

		tup := ConnTuple(ct.tup)
		cst := ConnStatsWithTimestamp(ct.conn_stats)
		tst := TCPStats(ct.tcp_stats)

		buffer = append(buffer, connStats(&tup, &cst, &tst))
		current += C.sizeof_conn_t
	}

	return buffer
}

func (p *PerfBatchManager) cleanupExpiredState(now time.Time) {
	for cpu := 0; cpu < len(p.stateByCPU); cpu++ {
		cpuState := &p.stateByCPU[cpu]
		for id, s := range cpuState.processed {
			if now.Sub(s.updated) >= p.expiredStateInterval {
				delete(cpuState.processed, id)
			}
		}
	}
}
