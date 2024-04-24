package npschedulerimpl

import (
	"sync"
	time "time"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

var timeNow = time.Now

// flowContext contains flow information and additional flush related data
type flowContext struct {
	flow                *pathtestConfig
	nextFlush           time.Time
	lastSuccessfulFlush time.Time
}

// flowAccumulator is used to accumulate aggregated pathtestConfigs
type flowAccumulator struct {
	pathtestConfigs map[uint64]flowContext
	// mutex is needed to protect `pathtestConfigs` since `flowAccumulator.add()` and  `flowAccumulator.flush()`
	// are called by different routines.
	pathtestConfigsMutex sync.Mutex

	flowFlushInterval time.Duration
	flowContextTTL    time.Duration

	logger log.Component
}

func newFlowContext(flow *pathtestConfig) flowContext {
	now := timeNow()
	return flowContext{
		flow:      flow,
		nextFlush: now,
	}
}

func newFlowAccumulator(aggregatorFlushInterval time.Duration, aggregatorFlowContextTTL time.Duration, logger log.Component) *flowAccumulator {
	return &flowAccumulator{
		pathtestConfigs:   make(map[uint64]flowContext),
		flowFlushInterval: aggregatorFlushInterval,
		flowContextTTL:    aggregatorFlowContextTTL,
		logger:            logger,
	}
}

// flush will flush specific flow context (distinct hash) if nextFlush is reached
// once a flow context is flushed nextFlush will be updated to the next flush time
//
// flowContextTTL:
// flowContextTTL defines the duration we should keep a specific flowContext in `flowAccumulator.pathtestConfigs`
// after `lastSuccessfulFlush`. // Flow context in `flowAccumulator.pathtestConfigs` map will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
// We need to keep flowContext (contains `nextFlush` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flowFlushInterval`).
// Example, after a flush, flowContext will have a new nextFlush, that will be the next flush time for new pathtestConfigs being added.
func (f *flowAccumulator) flush() []*pathtestConfig {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	f.logger.Tracef("f.pathtestConfigs: %+v", f.pathtestConfigs)
	// DEBUG STATEMENTS
	for _, ptConf := range f.pathtestConfigs {
		if ptConf.flow != nil {
			f.logger.Tracef("in-mem ptConf %s:%d", ptConf.flow.hostname, ptConf.flow.port)
		}
	}

	var flowsToFlush []*pathtestConfig
	for key, ptConfigCtx := range f.pathtestConfigs {
		now := timeNow()
		if ptConfigCtx.flow == nil && (ptConfigCtx.lastSuccessfulFlush.Add(f.flowContextTTL).Before(now)) {
			f.logger.Tracef("Delete flow context (key=%d, lastSuccessfulFlush=%s, nextFlush=%s)", key, ptConfigCtx.lastSuccessfulFlush.String(), ptConfigCtx.nextFlush.String())
			// delete ptConfigCtx wrapper if there is no successful flushes since `flowContextTTL`
			delete(f.pathtestConfigs, key)
			continue
		}
		if ptConfigCtx.nextFlush.After(now) {
			continue
		}
		if ptConfigCtx.flow != nil {
			flowsToFlush = append(flowsToFlush, ptConfigCtx.flow)
			ptConfigCtx.lastSuccessfulFlush = now
			ptConfigCtx.flow = nil
		}
		ptConfigCtx.nextFlush = ptConfigCtx.nextFlush.Add(f.flowFlushInterval)
		f.pathtestConfigs[key] = ptConfigCtx
	}
	return flowsToFlush
}

func (f *flowAccumulator) add(flowToAdd *pathtestConfig) {
	f.logger.Tracef("Add new flow: %+v", flowToAdd)

	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	aggHash := flowToAdd.AggregationHash()
	aggFlow, ok := f.pathtestConfigs[aggHash]
	if !ok {
		f.pathtestConfigs[aggHash] = newFlowContext(flowToAdd)
		return
	}
	if aggFlow.flow == nil {
		aggFlow.flow = flowToAdd
	} else {
		// accumulate flowToAdd with existing flow(s) with same hash
		//aggFlow.flow.Bytes += flowToAdd.Bytes
		f.logger.Warn("Extend TTL of pathtestConfig here")
	}
	f.pathtestConfigs[aggHash] = aggFlow
}

func (f *flowAccumulator) getFlowContextCount() int {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	return len(f.pathtestConfigs)
}
