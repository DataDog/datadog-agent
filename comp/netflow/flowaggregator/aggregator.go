// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package flowaggregator defines tools for aggregating observed netflows.
package flowaggregator

import (
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/netflow/format"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
)

const flushFlowsToSendInterval = 10 * time.Second
const metricPrefix = "datadog.netflow."

// FlowAggregator is used for space and time aggregation of NetFlow flows
type FlowAggregator struct {
	flowIn                       chan *common.Flow
	FlushFlowsToSendInterval     time.Duration // interval for checking flows to flush and send them to EP Forwarder
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      *flowAccumulator
	sender                       sender.Sender
	epForwarder                  eventplatform.Forwarder
	stopChan                     chan struct{}
	flushLoopDone                chan struct{}
	runDone                      chan struct{}
	ReceivedFlowCount            *atomic.Uint64
	FlushedFlowCount             *atomic.Uint64
	hostname                     string
	goflowPrometheusGatherer     prometheus.Gatherer
	TimeNowFunction              func() time.Time // Allows to mock time in tests
	dropFlowsBeforeAggregator    bool             // config option to drop flows before aggregation for performance testing
	dropFlowsBeforeEPForwarder   bool             // config option to drop flows before sending to EP forwarder for performance testing
	getMemoryStats               bool             // config option to enable memory statistics collection and metrics

	lastSequencePerExporter   map[sequenceDeltaKey]uint32
	lastSequencePerExporterMu sync.Mutex

	logger log.Component
}

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

// NewFlowAggregator returns a new FlowAggregator
func NewFlowAggregator(sender sender.Sender, epForwarder eventplatform.Forwarder, config *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) *FlowAggregator {
	flushInterval := time.Duration(config.AggregatorFlushInterval) * time.Second
	flowContextTTL := time.Duration(config.AggregatorFlowContextTTL) * time.Second
	rollupTrackerRefreshInterval := time.Duration(config.AggregatorRollupTrackerRefreshInterval) * time.Second
	return &FlowAggregator{
		flowIn:                       make(chan *common.Flow, config.AggregatorBufferSize),
		flowAcc:                      newFlowAccumulator(flushInterval, flowContextTTL, config.AggregatorPortRollupThreshold, config.AggregatorPortRollupDisabled, config.SkipHashCollisionDetection, config.AggregationHashUseSyncPool, config.PortRollupUseFixedSizeKey, config.PortRollupUseSingleStore, config.GetMemoryStats, config.GetCodeTimings, config.LogMapSizesEveryN, logger, rdnsQuerier),
		FlushFlowsToSendInterval:     flushFlowsToSendInterval,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       sender,
		epForwarder:                  epForwarder,
		stopChan:                     make(chan struct{}),
		runDone:                      make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		ReceivedFlowCount:            atomic.NewUint64(0),
		FlushedFlowCount:             atomic.NewUint64(0),
		hostname:                     hostname,
		goflowPrometheusGatherer:     prometheus.DefaultGatherer,
		TimeNowFunction:              time.Now,
		dropFlowsBeforeAggregator:    config.DropFlowsBeforeAggregator,
		dropFlowsBeforeEPForwarder:   config.DropFlowsBeforeEPForwarder,
		getMemoryStats:               config.GetMemoryStats,
		lastSequencePerExporter:      make(map[sequenceDeltaKey]uint32),
		logger:                       logger,
	}
}

// Start will start the FlowAggregator worker
func (agg *FlowAggregator) Start() {
	agg.logger.Info("Flow Aggregator started")
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
	var droppedFlowCount uint64
	for {
		select {
		case <-agg.stopChan:
			if agg.dropFlowsBeforeAggregator && droppedFlowCount > 0 {
				agg.logger.Infof("Dropped %d flows before aggregator as configured for performance testing", droppedFlowCount)
			}
			agg.logger.Info("Stopping aggregator")
			agg.runDone <- struct{}{}
			return
		case flow := <-agg.flowIn:
			agg.ReceivedFlowCount.Inc()
			if agg.dropFlowsBeforeAggregator {
				droppedFlowCount++
				// Log every 1000000 dropped flows for visibility
				if droppedFlowCount%1000000 == 0 {
					agg.logger.Infof("Dropped %d flows before aggregator (performance testing mode)", droppedFlowCount)
				}
				return
			}
			agg.flowAcc.add(flow)
		}
	}
}

func (agg *FlowAggregator) sendFlows(flows []*common.Flow, flushTime time.Time) {
	for _, flow := range flows {
		flowPayload := buildPayload(flow, agg.hostname, flushTime)

		// Calling MarshalJSON directly as it's faster than calling json.Marshall
		payloadBytes, err := flowPayload.MarshalJSON()
		if err != nil {
			agg.logger.Errorf("Error marshalling device metadata: %s", err)
			continue
		}
		agg.logger.Tracef("flushed flow: %s", string(payloadBytes))

		m := message.NewMessage(payloadBytes, nil, "", 0)
		if !agg.dropFlowsBeforeEPForwarder {
			// JMWPERF if tghis blocks due to channel being full, does it block processing of incoming flows?
			err = agg.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesNetFlow)
			if err != nil {
				// at the moment, SendEventPlatformEventBlocking can only fail if the event type is invalid
				agg.logger.Errorf("Error sending to event platform forwarder: %s", err)
				continue
			}
		}
	}
	if agg.dropFlowsBeforeEPForwarder {
		agg.logger.Infof("Dropped %d flows before EP forwarder as configured for performance testing", len(flows))
	}
}

func (agg *FlowAggregator) sendExporterMetadata(flows []*common.Flow, flushTime time.Time) {
	// exporterMap structure: map[NAMESPACE]map[EXPORTER_ID]metadata.NetflowExporter
	exporterMap := make(map[string]map[string]metadata.NetflowExporter)

	// orderedExporterIDs is used to build predictable metadata payload (consistent batches and orders)
	// orderedExporterIDs structure: map[NAMESPACE][]EXPORTER_ID
	orderedExporterIDs := make(map[string][]string)

	for _, flow := range flows {
		exporterIPAddress := format.IPAddr(flow.ExporterAddr)
		if exporterIPAddress == "" || strings.HasPrefix(exporterIPAddress, "?") {
			agg.logger.Errorf("Invalid exporter Addr: %s", exporterIPAddress)
			continue
		}
		exporterID := flow.Namespace + ":" + exporterIPAddress + ":" + string(flow.FlowType)
		if _, ok := exporterMap[flow.Namespace]; !ok {
			exporterMap[flow.Namespace] = make(map[string]metadata.NetflowExporter)
		}
		if _, ok := exporterMap[flow.Namespace][exporterID]; ok {
			// this exporter is already in the map, no need to reprocess it
			continue
		}
		exporterMap[flow.Namespace][exporterID] = metadata.NetflowExporter{
			ID:        exporterID,
			IPAddress: exporterIPAddress,
			FlowType:  string(flow.FlowType),
		}
		orderedExporterIDs[flow.Namespace] = append(orderedExporterIDs[flow.Namespace], exporterID)
	}
	for namespace, ids := range orderedExporterIDs {
		var netflowExporters []metadata.NetflowExporter
		for _, exporterID := range ids {
			netflowExporters = append(netflowExporters, exporterMap[namespace][exporterID])
		}
		metadataPayloads := metadata.BatchPayloads(integrations.Netflow, namespace, "", flushTime, metadata.PayloadMetadataBatchSize, nil, nil, nil, nil, nil, netflowExporters, nil)
		for _, payload := range metadataPayloads {
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				agg.logger.Errorf("Error marshalling device metadata: %s", err)
				continue
			}
			agg.logger.Debugf("netflow exporter metadata payload: %s", string(payloadBytes))
			m := message.NewMessage(payloadBytes, nil, "", 0)
			if !agg.dropFlowsBeforeEPForwarder {
				err = agg.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata)
				if err != nil {
					agg.logger.Errorf("Error sending event platform event for netflow exporter metadata: %s", err)
				}
			}
		}
	}
	if agg.dropFlowsBeforeEPForwarder {
		agg.logger.Infof("Dropped exporter metadata for %d flows before EP forwarder as configured for performance testing", len(flows))
	}
}

func (agg *FlowAggregator) flushLoop() {
	var flushFlowsToSendTicker <-chan time.Time

	if agg.FlushFlowsToSendInterval > 0 {
		flushTicker := time.NewTicker(agg.FlushFlowsToSendInterval)
		flushFlowsToSendTicker = flushTicker.C
		defer flushTicker.Stop()
	} else {
		agg.logger.Debug("flushFlowsToSendInterval set to 0: will never flush automatically")
	}

	rollupTicker := time.NewTicker(agg.rollupTrackerRefreshInterval)
	defer rollupTicker.Stop()
	rollupTrackersRefresh := rollupTicker.C
	// TODO: move rollup tracker refresh to a separate loop (separate PR) to avoid rollup tracker and flush flows impacting each other

	var lastFlushTime time.Time
	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			agg.flushLoopDone <- struct{}{}
			return
		// automatic flush sequence
		case flushStartTime := <-flushFlowsToSendTicker:
			if !lastFlushTime.IsZero() {
				flushInterval := flushStartTime.Sub(lastFlushTime)
				agg.sender.Gauge("datadog.netflow.aggregator.flush_interval", flushInterval.Seconds(), "", nil)
			}
			lastFlushTime = flushStartTime
			agg.flush()
			agg.sender.Gauge("datadog.netflow.aggregator.flush_duration", time.Since(flushStartTime).Seconds(), "", nil)
			agg.sender.Commit()
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

	flowsToFlush, flowAccStats := agg.flowAcc.flush()
	agg.sender.Gauge("datadog.netflow.aggregator.perf_flowacc_flush_duration", time.Since(flushTime).Seconds(), "", nil)

	agg.logger.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

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
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.ReceivedFlowCount.Load()), "", nil)
	agg.sender.Count("datadog.netflow.aggregator.flows_flushed", float64(flushCount), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.flows_contexts", float64(flowsContexts), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_size", float64(agg.flowAcc.portRollup.GetCurrentStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_size", float64(agg.flowAcc.portRollup.GetNewStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_ipv4_size", float64(agg.flowAcc.portRollup.GetCurrentStoreSizeIPv4()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_ipv4_size", float64(agg.flowAcc.portRollup.GetNewStoreSizeIPv4()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.single_store_size", float64(agg.flowAcc.portRollup.GetSingleStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.single_store_ipv4_size", float64(agg.flowAcc.portRollup.GetSingleStoreSizeIPv4()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.capacity", float64(cap(agg.flowIn)), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.length", float64(len(agg.flowIn)), "", nil)

	if flowAccStats.flowAccAddCount > 0 {
		agg.sender.Count("datadog.netflow.aggregator.perf_flow_acc_add_count", float64(flowAccStats.flowAccAddCount), "", nil)
		agg.sender.Count("datadog.netflow.aggregator.perf_flow_acc_add_duration", float64(flowAccStats.flowAccAddDurationSec), "", nil)
	}
	if flowAccStats.getAggregationHashCount > 0 {
		agg.sender.Count("datadog.netflow.aggregator.perf_get_aggregation_hash_count", float64(flowAccStats.getAggregationHashCount), "", nil)
		agg.sender.Count("datadog.netflow.aggregator.perf_get_aggregation_hash_duration_nanonow", float64(flowAccStats.getAggregationHashDurationSecNanoNow), "", nil)
		agg.sender.Count("datadog.netflow.aggregator.perf_get_aggregation_hash_duration_unixnano", float64(flowAccStats.getAggregationHashDurationSecUnixNano), "", nil)
	}
	if flowAccStats.portRollupAddCount > 0 {
		agg.sender.Count("datadog.netflow.aggregator.perf_port_rollup_add_count", float64(flowAccStats.portRollupAddCount), "", nil)
		agg.sender.Count("datadog.netflow.aggregator.perf_port_rollup_add_duration", float64(flowAccStats.portRollupAddDurationSec), "", nil)
	}
	if flowAccStats.flowSizeBytes > 0 {
		agg.sender.Count("datadog.netflow.aggregator.perf_flow_size_count", float64(flowAccStats.flowSizeCount), "", nil)
		agg.sender.Count("datadog.netflow.aggregator.perf_flow_size_bytes", float64(flowAccStats.flowSizeBytes), "", nil)
	}

	err := agg.submitCollectorMetrics()
	if err != nil {
		agg.logger.Warnf("error submitting collector metrics: %s", err)
	}

	// We increase `FlushedFlowCount` at the end to be sure that the metrics are submitted before hand.
	// Tests will wait for `FlushedFlowCount` to be increased before asserting the metrics.
	agg.FlushedFlowCount.Add(uint64(flushCount))
	return len(flowsToFlush)
}

// getSequenceDelta return the delta of current sequence number compared to previously saved sequence number
// Since we track per exporterIP, the returned delta is only accurate when for the specific exporterIP there is
// only one NetFlow9/IPFIX observation domain, NetFlow5 engineType/engineId, sFlow agent/subagent.
func (agg *FlowAggregator) getSequenceDelta(flowsToFlush []*common.Flow) map[sequenceDeltaKey]sequenceDeltaValue {
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
		agg.logger.Debugf("[getSequenceDelta] key=%s, seqnum=%d, delta=%d, last=%d, reset=%t", key, seqnum, delta, agg.lastSequencePerExporter[key], reset)
		seqDeltaValue := sequenceDeltaValue{LastSequence: seqnum}
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
	agg.logger.Debugf("Rollup tracker refresh: use new store as current store")
	start := timeNow()
	agg.flowAcc.portRollup.UseNewStoreAsCurrentStore()
	agg.sender.Count("datadog.netflow.aggregator.perf_rollup_tracker_refresh_duration", float64(time.Since(start).Seconds()), "", nil)
}

func (agg *FlowAggregator) submitCollectorMetrics() error {
	promMetrics, err := agg.goflowPrometheusGatherer.Gather()
	if err != nil {
		return err
	}
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			agg.logger.Tracef("Collector metric `%s`: type=`%v` value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := goflowlib.ConvertMetric(metric, metricFamily)
			if err != nil {
				agg.logger.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				agg.sender.Gauge(metricPrefix+name, value, "", tags)
			case metrics.MonotonicCountType:
				agg.sender.MonotonicCount(metricPrefix+name, value, "", tags)
			default:
				agg.logger.Debugf("cannot submit unsupported type %s", metricType.String())
			}
		}
	}
	return nil
}
