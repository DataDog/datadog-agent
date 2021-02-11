// +build linux_bpf

package tracer

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"

	"github.com/DataDog/ebpf"
)

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
	stateByCPU []batchState

	// maxIdleInterval represents the maximum time (in nanoseconds)
	// a batch can remain idle (that is, without being flushed) on eBPF
	maxIdleInterval int64
}

// NewPerfBatchManager returns a new `PerfBatchManager` and initializes the
// eBPF map that holds the tcp_close batch objects.
func NewPerfBatchManager(batchMap *ebpf.Map, maxIdleInterval time.Duration, numBatches int) (*PerfBatchManager, error) {
	if batchMap == nil {
		return nil, fmt.Errorf("batchMap is nil")
	}

	for i := 0; i < numBatches; i++ {
		b := new(batch)
		b.cpu = _Ctype_ushort(i)
		batchMap.Put(unsafe.Pointer(&i), unsafe.Pointer(b))
	}

	return &PerfBatchManager{
		batchMap:        batchMap,
		stateByCPU:      make([]batchState, numBatches),
		maxIdleInterval: maxIdleInterval.Nanoseconds(),
	}, nil
}

// Extract from the given batch all connections that haven't been processed yet.
// This method is also responsible for keeping track of each CPU core batch state.
func (p *PerfBatchManager) Extract(b *batch, now time.Time) []network.ConnectionStats {
	if int(b.cpu) >= len(p.stateByCPU) {
		return nil
	}

	state := &p.stateByCPU[b.cpu]
	lastOffset := state.offset
	state.updated = now.UnixNano()
	state.offset = 0

	buffer := make([]network.ConnectionStats, 0, ConnCloseBatchSize)
	return ExtractBatchInto(buffer, b, lastOffset, ConnCloseBatchSize)
}

// GetIdleConns return all connections that have been "stuck" in idle batches
// for more than `idleInterval`
func (p *PerfBatchManager) GetIdleConns(now time.Time) []network.ConnectionStats {
	var idle []network.ConnectionStats
	nowTS := now.UnixNano()
	batch := new(batch)
	for i := 0; i < len(p.stateByCPU); i++ {
		state := &p.stateByCPU[i]

		if (nowTS - state.updated) < p.maxIdleInterval {
			continue
		}

		// we have an idle batch, so let's retrieve its data from eBPF
		err := p.batchMap.Lookup(unsafe.Pointer(&i), unsafe.Pointer(batch))
		if err != nil {
			continue
		}

		pos := int(batch.pos)
		if pos == 0 {
			continue
		}

		state.updated = nowTS
		if pos == state.offset {
			continue
		}

		idle = ExtractBatchInto(idle, batch, state.offset, pos)
		state.offset = pos
	}

	return idle
}

type batchState struct {
	offset  int
	updated int64
}
