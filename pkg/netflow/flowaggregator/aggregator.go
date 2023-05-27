// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
)

const flushFlowsToSendInterval = 10 * time.Second
const metricPrefix = "datadog.netflow."

// FlowAggregator is used for space and time aggregation of NetFlow flows
type FlowAggregator struct {
	flowIn                       chan *common.Flow
	flushFlowsToSendInterval     time.Duration // interval for checking flows to flush and send them to EP Forwarder
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      *flowAccumulator
	sender                       aggregator.Sender
	epForwarder                  epforwarder.EventPlatformForwarder
	stopChan                     chan struct{}
	flushLoopDone                chan struct{}
	runDone                      chan struct{}
	receivedFlowCount            *atomic.Uint64
	flushedFlowCount             *atomic.Uint64
	hostname                     string
	goflowPrometheusGatherer     prometheus.Gatherer
	TimeNowFunction              func() time.Time // Allows to mock time in tests

	lastSequencePerExporter   map[SequenceDeltaKey]uint32
	lastSequencePerExporterMu sync.Mutex
}

type SequenceDeltaKey struct {
	Namespace  string
	ExporterIP string
	FlowType   common.FlowType
}

type SequenceDeltaValue struct {
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

// NewFlowAggregator returns a new FlowAggregator
func NewFlowAggregator(sender aggregator.Sender, epForwarder epforwarder.EventPlatformForwarder, config *config.NetflowConfig, hostname string) *FlowAggregator {
	flushInterval := time.Duration(config.AggregatorFlushInterval) * time.Second
	flowContextTTL := time.Duration(config.AggregatorFlowContextTTL) * time.Second
	rollupTrackerRefreshInterval := time.Duration(config.AggregatorRollupTrackerRefreshInterval) * time.Second
	return &FlowAggregator{
		flowIn:                       make(chan *common.Flow, config.AggregatorBufferSize),
		flowAcc:                      newFlowAccumulator(flushInterval, flowContextTTL, config.AggregatorPortRollupThreshold, config.AggregatorPortRollupDisabled),
		flushFlowsToSendInterval:     flushFlowsToSendInterval,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       sender,
		epForwarder:                  epForwarder,
		stopChan:                     make(chan struct{}),
		runDone:                      make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		receivedFlowCount:            atomic.NewUint64(0),
		flushedFlowCount:             atomic.NewUint64(0),
		hostname:                     hostname,
		goflowPrometheusGatherer:     prometheus.DefaultGatherer,
		TimeNowFunction:              time.Now,
		lastSequencePerExporter:      make(map[SequenceDeltaKey]uint32),
	}
}

// Start will start the FlowAggregator worker
func (agg *FlowAggregator) Start() {
	log.Info("Flow Aggregator started")
	go agg.run()
	agg.flushLoop() // blocking call
}

// Stop will stop running FlowAggregator
func (agg *FlowAggregator) Stop() {
	close(agg.stopChan)
	<-agg.flushLoopDone
	<-agg.runDone
}

// GetFlowInChan returns flow input chan
func (agg *FlowAggregator) GetFlowInChan() chan *common.Flow {
	return agg.flowIn
}

func (agg *FlowAggregator) run() {
	for {
		select {
		case <-agg.stopChan:
			log.Info("Stopping aggregator")
			agg.runDone <- struct{}{}
			return
		case flow := <-agg.flowIn:
			agg.receivedFlowCount.Inc()
			agg.flowAcc.add(flow)
		}
	}
}

func (agg *FlowAggregator) sendFlows(flows []*common.Flow, flushTime time.Time) {
	for _, flow := range flows {
		flowPayload := buildPayload(flow, agg.hostname, flushTime)
		payloadBytes, err := json.Marshal(flowPayload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			continue
		}

		log.Tracef("flushed flow: %s", string(payloadBytes))

		m := &message.Message{Content: payloadBytes}
		err = agg.epForwarder.SendEventPlatformEventBlocking(m, epforwarder.EventTypeNetworkDevicesNetFlow)
		if err != nil {
			// at the moment, SendEventPlatformEventBlocking can only fail if the event type is invalid
			log.Errorf("Error sending to event platform forwarder: %s", err)
			continue
		}
	}
}

func (agg *FlowAggregator) sendExporterMetadata(flows []*common.Flow, flushTime time.Time) {
	// exporterMap structure: map[NAMESPACE]map[EXPORTER_ID]metadata.NetflowExporter
	exporterMap := make(map[string]map[string]metadata.NetflowExporter)

	// orderedExporterIDs is used to build predictable metadata payload (consistent batches and orders)
	// orderedExporterIDs structure: map[NAMESPACE][]EXPORTER_ID
	orderedExporterIDs := make(map[string][]string)

	for _, flow := range flows {
		exporterIpAddress := common.IPBytesToString(flow.ExporterAddr)
		if exporterIpAddress == "" || strings.HasPrefix(exporterIpAddress, "?") {
			log.Errorf("Invalid exporter Addr: %s", exporterIpAddress)
			continue
		}
		exporterID := flow.Namespace + ":" + exporterIpAddress + ":" + string(flow.FlowType)
		if _, ok := exporterMap[flow.Namespace]; !ok {
			exporterMap[flow.Namespace] = make(map[string]metadata.NetflowExporter)
		}
		if _, ok := exporterMap[flow.Namespace][exporterID]; ok {
			// this exporter is already in the map, no need to reprocess it
			continue
		}
		exporterMap[flow.Namespace][exporterID] = metadata.NetflowExporter{
			ID:        exporterID,
			IPAddress: exporterIpAddress,
			FlowType:  string(flow.FlowType),
		}
		orderedExporterIDs[flow.Namespace] = append(orderedExporterIDs[flow.Namespace], exporterID)
	}
	for namespace, ids := range orderedExporterIDs {
		var netflowExporters []metadata.NetflowExporter
		for _, exporterId := range ids {
			netflowExporters = append(netflowExporters, exporterMap[namespace][exporterId])
		}
		metadataPayloads := metadata.BatchPayloads(namespace, "", flushTime, metadata.PayloadMetadataBatchSize, nil, nil, nil, nil, netflowExporters)
		for _, payload := range metadataPayloads {
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				log.Errorf("Error marshalling device metadata: %s", err)
				continue
			}
			log.Debugf("netflow exporter metadata payload: %s", string(payloadBytes))
			m := &message.Message{Content: payloadBytes}
			err = agg.epForwarder.SendEventPlatformEventBlocking(m, epforwarder.EventTypeNetworkDevicesMetadata)
			if err != nil {
				log.Errorf("Error sending event platform event for netflow exporter metadata: %s", err)
			}
		}
	}
}

func (agg *FlowAggregator) flushLoop() {
	var flushFlowsToSendTicker <-chan time.Time

	if agg.flushFlowsToSendInterval > 0 {
		flushFlowsToSendTicker = time.NewTicker(agg.flushFlowsToSendInterval).C
	} else {
		log.Debug("flushFlowsToSendInterval set to 0: will never flush automatically")
	}

	rollupTrackersRefresh := time.NewTicker(agg.rollupTrackerRefreshInterval).C
	// TODO: move rollup tracker refresh to a separate loop (separate PR) to avoid rollup tracker and flush flows impacting each other

	var lastFlushTime time.Time
	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			agg.flushLoopDone <- struct{}{}
			return
		// automatic flush sequence
		case <-flushFlowsToSendTicker:
			now := time.Now()
			if !lastFlushTime.IsZero() {
				flushInterval := now.Sub(lastFlushTime)
				agg.sender.Gauge("datadog.netflow.aggregator.flush_interval", flushInterval.Seconds(), "", nil)
			}
			lastFlushTime = now

			flushStartTime := time.Now()
			agg.flush()
			agg.sender.Gauge("datadog.netflow.aggregator.flush_duration", time.Since(flushStartTime).Seconds(), "", nil)
		// refresh rollup trackers
		case <-rollupTrackersRefresh:
			agg.rollupTrackersRefresh()
		}
	}
}

// Flush flushes the aggregator
func (agg *FlowAggregator) flush() int {
	flowsContexts := agg.flowAcc.getFlowContextCount()
	flushTime := agg.TimeNowFunction()
	flowsToFlush := agg.flowAcc.flush()
	log.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

	sequenceDeltaPerExporter := agg.getSequenceDelta(flowsToFlush)
	for key, seqDelta := range sequenceDeltaPerExporter {
		tags := []string{"device_namespace:" + key.Namespace, "exporter_ip:" + key.ExporterIP, "flow_type:" + string(key.FlowType)}
		agg.sender.Count("datadog.netflow.aggregator.sequence.delta", float64(seqDelta.Delta), "", tags)
		agg.sender.Gauge("datadog.netflow.aggregator.sequence.last", float64(seqDelta.LastSequence), "", tags)
		if seqDelta.Reset {
			agg.sender.Count("datadog.netflow.aggregator.sequence.reset", float64(1), "", tags)
		}
	}

	// TODO: Add flush stats to agent telemetry e.g. aggregator newFlushCountStats()
	if len(flowsToFlush) > 0 {
		agg.sendFlows(flowsToFlush, flushTime)
	}
	agg.sendExporterMetadata(flowsToFlush, flushTime)

	flushCount := len(flowsToFlush)

	agg.sender.MonotonicCount("datadog.netflow.aggregator.hash_collisions", float64(agg.flowAcc.hashCollisionFlowCount.Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.Count("datadog.netflow.aggregator.flows_flushed", float64(flushCount), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.flows_contexts", float64(flowsContexts), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_size", float64(agg.flowAcc.portRollup.GetCurrentStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_size", float64(agg.flowAcc.portRollup.GetNewStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.capacity", float64(cap(agg.flowIn)), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.length", float64(len(agg.flowIn)), "", nil)

	err := agg.submitCollectorMetrics()
	if err != nil {
		log.Warnf("error submitting collector metrics: %s", err)
	}

	// We increase `flushedFlowCount` at the end to be sure that the metrics are submitted before hand.
	// Tests will wait for `flushedFlowCount` to be increased before asserting the metrics.
	agg.flushedFlowCount.Add(uint64(flushCount))
	return len(flowsToFlush)
}

// getSequenceDelta return the delta of current sequence number compared to previously saved sequence number
// Since we track per exporterIP, the returned delta is only accurate when for the specific exporterIP there is
// only one NetFlow9/IPFIX observation domain, NetFlow5 engineType/engineId, sFlow agent/subagent.
func (agg *FlowAggregator) getSequenceDelta(flowsToFlush []*common.Flow) map[SequenceDeltaKey]SequenceDeltaValue {
	maxSequencePerExporter := make(map[SequenceDeltaKey]uint32)
	for _, flow := range flowsToFlush {
		key := SequenceDeltaKey{
			Namespace:  flow.Namespace,
			ExporterIP: net.IP(flow.ExporterAddr).String(),
			FlowType:   flow.FlowType,
		}
		if flow.SequenceNum > maxSequencePerExporter[key] {
			maxSequencePerExporter[key] = flow.SequenceNum
		}
	}
	sequenceDeltaPerExporter := make(map[SequenceDeltaKey]SequenceDeltaValue)

	agg.lastSequencePerExporterMu.Lock()
	defer agg.lastSequencePerExporterMu.Unlock()
	for key, seqnum := range maxSequencePerExporter {
		lastSeq, prevExist := agg.lastSequencePerExporter[key]
		delta := int64(0)
		if prevExist {
			delta = int64(seqnum) - int64(lastSeq)
		}
		maxNegSeqDiff := maxNegativeSequenceDiffToReset[key.FlowType]
		reset := delta < int64(maxNegSeqDiff)
		log.Debugf("[getSequenceDelta] key=%s, seqnum=%d, delta=%d, last=%d, reset=%t", key, seqnum, delta, agg.lastSequencePerExporter[key], reset)
		seqDeltaValue := SequenceDeltaValue{LastSequence: seqnum}
		if reset { // sequence reset
			seqDeltaValue.Delta = int64(seqnum)
			seqDeltaValue.Reset = reset
			agg.lastSequencePerExporter[key] = seqnum
		} else if delta < 0 {
			seqDeltaValue.Delta = 0
		} else {
			seqDeltaValue.Delta = delta
			agg.lastSequencePerExporter[key] = seqnum
		}
		sequenceDeltaPerExporter[key] = seqDeltaValue
	}
	return sequenceDeltaPerExporter
}

func (agg *FlowAggregator) rollupTrackersRefresh() {
	log.Debugf("Rollup tracker refresh: use new store as current store")
	agg.flowAcc.portRollup.UseNewStoreAsCurrentStore()
}

func (agg *FlowAggregator) submitCollectorMetrics() error {
	promMetrics, err := agg.goflowPrometheusGatherer.Gather()
	if err != nil {
		return err
	}
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			log.Tracef("Collector metric `%s`: type=`%v` value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := goflowlib.ConvertMetric(metric, metricFamily)
			if err != nil {
				log.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				agg.sender.Gauge(metricPrefix+name, value, "", tags)
			case metrics.MonotonicCountType:
				agg.sender.MonotonicCount(metricPrefix+name, value, "", tags)
			default:
				log.Debugf("cannot submit unsupported type %s", metricType.String())
			}
		}
	}
	return nil
}
