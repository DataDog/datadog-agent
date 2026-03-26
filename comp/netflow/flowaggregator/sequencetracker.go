// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"net"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

type sequenceDeltaKey struct {
	Namespace  string
	ExporterIP string
	FlowType   common.FlowType
}

type sequenceDeltaValue struct {
	Delta        int64
	LastSequence uint32
	Reset        bool
}

// maxNegativeSequenceDiffToReset are thresholds used to detect sequence reset
var maxNegativeSequenceDiffToReset = map[common.FlowType]int{
	common.TypeSFlow5:   -1000,
	common.TypeNetFlow5: -1000,
	common.TypeNetFlow9: -100,
	common.TypeIPFIX:    -100,
}

// sequenceTracker tracks per-exporter sequence deltas across flush cycles.
type sequenceTracker struct {
	lastSequencePerExporter   map[sequenceDeltaKey]uint32
	lastSequencePerExporterMu sync.Mutex
	sender                    sender.Sender
	logger                    log.Component
}

func newSequenceTracker(snd sender.Sender, logger log.Component) *sequenceTracker {
	return &sequenceTracker{
		lastSequencePerExporter: make(map[sequenceDeltaKey]uint32),
		sender:                  snd,
		logger:                  logger,
	}
}

// trackAndEmit computes sequence deltas for the given flows and emits metrics.
func (st *sequenceTracker) trackAndEmit(flows []*common.Flow) {
	deltas := st.getSequenceDelta(flows)
	for key, seqDelta := range deltas {
		tags := []string{"device_namespace:" + key.Namespace, "exporter_ip:" + key.ExporterIP, "flow_type:" + string(key.FlowType)}
		st.sender.Count("datadog.netflow.aggregator.sequence.delta", float64(seqDelta.Delta), "", tags)
		st.sender.Gauge("datadog.netflow.aggregator.sequence.last", float64(seqDelta.LastSequence), "", tags)
		if seqDelta.Reset {
			st.sender.Count("datadog.netflow.aggregator.sequence.reset", float64(1), "", tags)
		}
	}
}

// getSequenceDelta returns the delta of current sequence number compared to previously saved sequence number.
// Since we track per exporterIP, the returned delta is only accurate when for the specific exporterIP there is
// only one NetFlow9/IPFIX observation domain, NetFlow5 engineType/engineId, sFlow agent/subagent.
func (st *sequenceTracker) getSequenceDelta(flowsToFlush []*common.Flow) map[sequenceDeltaKey]sequenceDeltaValue {
	maxSequencePerExporter := make(map[sequenceDeltaKey]uint32)
	for _, flow := range flowsToFlush {
		key := sequenceDeltaKey{
			Namespace:  flow.Namespace,
			ExporterIP: net.IP(flow.ExporterAddr).String(),
			FlowType:   flow.FlowType,
		}
		if flow.SequenceNum > maxSequencePerExporter[key] {
			maxSequencePerExporter[key] = flow.SequenceNum
		}
	}
	sequenceDeltaPerExporter := make(map[sequenceDeltaKey]sequenceDeltaValue)

	st.lastSequencePerExporterMu.Lock()
	defer st.lastSequencePerExporterMu.Unlock()
	for key, seqnum := range maxSequencePerExporter {
		lastSeq, prevExist := st.lastSequencePerExporter[key]
		delta := int64(0)
		if prevExist {
			delta = int64(seqnum) - int64(lastSeq)
		}
		maxNegSeqDiff := maxNegativeSequenceDiffToReset[key.FlowType]
		reset := delta < int64(maxNegSeqDiff)
		st.logger.Debugf("[getSequenceDelta] key=%s, seqnum=%d, delta=%d, last=%d, reset=%t", key, seqnum, delta, st.lastSequencePerExporter[key], reset)
		seqDeltaValue := sequenceDeltaValue{LastSequence: seqnum}
		if reset { // sequence reset
			seqDeltaValue.Delta = int64(seqnum)
			seqDeltaValue.Reset = reset
			st.lastSequencePerExporter[key] = seqnum
		} else if delta < 0 {
			seqDeltaValue.Delta = 0
		} else {
			seqDeltaValue.Delta = delta
			st.lastSequencePerExporter[key] = seqnum
		}
		sequenceDeltaPerExporter[key] = seqDeltaValue
	}
	return sequenceDeltaPerExporter
}
