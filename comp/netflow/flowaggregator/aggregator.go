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

	"github.com/DataDog/datadog-agent/comp/netflow/topn"
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

// FlowAggregatorRunner is the non-generic interface for running a FlowAggregator.
// Used by external consumers (server, listener) that don't need to know the
// accumulator's flush result type.
type FlowAggregatorRunner interface {
	Start()
	Stop()
	GetFlowInChan() chan *common.Flow
}

// FlowAggregator is used for space and time aggregation of NetFlow flows.
// The type parameter T is the flush result type from the accumulator:
//   - []*common.Flow for the standard path
//   - []FlowGroup for the dedup path
type FlowAggregator[T any] struct {
	flowIn                       chan *common.Flow
	FlushConfig                  common.FlushConfig
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      FlowAccumulator[T]
	// submit is wired at construction time to the appropriate send logic for T.
	submit  func(result T, ctx common.FlushContext) int
	sender  sender.Sender
	epForwarder                  eventplatform.Forwarder
	stopChan                     chan struct{}
	flushLoopDone                chan struct{}
	runDone                      chan struct{}
	receivedFlowCount            *atomic.Uint64
	flushedFlowCount             *atomic.Uint64
	hostname                     string
	goflowPrometheusGatherer     prometheus.Gatherer
	TimeNowFunction              func() time.Time // Allows to mock time in tests
	NewTicker                    func(duration time.Duration) <-chan time.Time

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

// FlowFlushFilter is an interface that can be used to filter flows before they are sent to the EP Forwarder.
type FlowFlushFilter interface {
	Filter(flushCtx common.FlushContext, flows []*common.Flow) []*common.Flow
}

// maxNegativeSequenceDiffToReset are thresholds used to detect sequence reset
var maxNegativeSequenceDiffToReset = map[common.FlowType]int{
	common.TypeSFlow5:   -1000,
	common.TypeNetFlow5: -1000,
	common.TypeNetFlow9: -100,
	common.TypeIPFIX:    -100,
}

// newFlowAggregatorBase builds the shared fields for a FlowAggregator[T].
func newFlowAggregatorBase[T any](flowAcc FlowAccumulator[T], flushConfig common.FlushConfig, rollupTrackerRefreshInterval time.Duration, bufferSize int, snd sender.Sender, epForwarder eventplatform.Forwarder, hostname string, logger log.Component) *FlowAggregator[T] {
	return &FlowAggregator[T]{
		flowIn:                       make(chan *common.Flow, bufferSize),
		flowAcc:                      flowAcc,
		FlushConfig:                  flushConfig,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       snd,
		epForwarder:                  epForwarder,
		stopChan:                     make(chan struct{}),
		runDone:                      make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		receivedFlowCount:            atomic.NewUint64(0),
		flushedFlowCount:             atomic.NewUint64(0),
		hostname:                     hostname,
		goflowPrometheusGatherer:     prometheus.DefaultGatherer,
		TimeNowFunction:              time.Now,
		NewTicker:                    time.Tick,
		lastSequencePerExporter:      make(map[sequenceDeltaKey]uint32),
		logger:                       logger,
	}
}

// NewStandardFlowAggregator creates a FlowAggregator that flushes individual flows.
// This is the standard (non-dedup) path with optional TopN filtering and jitter scheduling.
func NewStandardFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) *FlowAggregator[[]*common.Flow] {
	flushConfig := common.FlushConfig{
		FlowCollectionDuration: time.Duration(conf.AggregatorFlushInterval) * time.Second,
		FlushTickFrequency:     flushFlowsToSendInterval,
	}

	var topNFilter FlowFlushFilter = topn.NoopFilter{}
	var flowScheduler FlowScheduler = ImmediateFlowScheduler{flushConfig: flushConfig}
	if conf.AggregatorMaxFlowsPerPeriod > 0 {
		topNFilter = topn.NewPerFlushFilter(int64(conf.AggregatorMaxFlowsPerPeriod), flushConfig, snd, logger)
		flowScheduler = JitterFlowScheduler{flushConfig: flushConfig}
	}

	flowContextTTL := time.Duration(conf.AggregatorFlowContextTTL) * time.Second
	rollupInterval := time.Duration(conf.AggregatorRollupTrackerRefreshInterval) * time.Second

	acc := newStandardFlowAccumulator(flushConfig, flowScheduler, flowContextTTL, conf.AggregatorPortRollupThreshold, conf.AggregatorPortRollupDisabled, logger, rdnsQuerier)
	agg := newFlowAggregatorBase[[]*common.Flow](acc, flushConfig, rollupInterval, conf.AggregatorBufferSize, snd, epForwarder, hostname, logger)

	agg.submit = func(flows []*common.Flow, ctx common.FlushContext) int {
		// Apply TopN filtering.
		flowsBeforeFilter := len(flows)
		flows = topNFilter.Filter(ctx, flows)
		numRowsFiltered := flowsBeforeFilter - len(flows)

		agg.logger.Debugf("Flushing %d flows to the forwarder, %d dropped by TopN filtering (flush_duration=%d, flow_contexts_before_flush=%d)",
			len(flows), numRowsFiltered, time.Since(ctx.FlushTime).Milliseconds(), agg.flowAcc.GetFlowContextCount())

		agg.emitSequenceMetrics(agg.getSequenceDelta(flows))

		// TODO: Add flush stats to agent telemetry e.g. aggregator newFlushCountStats()
		if len(flows) > 0 {
			agg.sendFlows(flows, ctx.FlushTime)
		}
		agg.sendExporterMetadata(flows, ctx.FlushTime)
		return len(flows)
	}

	return agg
}

// NewDedupFlowAggregator creates a FlowAggregator that flushes grouped FlowGroups.
// This is the deduplication path: flows sharing a 5-tuple are merged into a single
// event with a reporters list. TopN filtering and jitter scheduling are disabled.
func NewDedupFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) *FlowAggregator[[]FlowGroup] {
	flushConfig := common.FlushConfig{
		FlowCollectionDuration: time.Duration(conf.AggregatorFlushInterval) * time.Second,
		FlushTickFrequency:     flushFlowsToSendInterval,
	}

	flowScheduler := ImmediateFlowScheduler{flushConfig: flushConfig}
	flowContextTTL := time.Duration(conf.AggregatorFlowContextTTL) * time.Second
	rollupInterval := time.Duration(conf.AggregatorRollupTrackerRefreshInterval) * time.Second

	acc := newDedupFlowAccumulator(flushConfig, flowScheduler, flowContextTTL, conf.AggregatorPortRollupThreshold, conf.AggregatorPortRollupDisabled, logger, rdnsQuerier)
	agg := newFlowAggregatorBase[[]FlowGroup](acc, flushConfig, rollupInterval, conf.AggregatorBufferSize, snd, epForwarder, hostname, logger)

	agg.submit = func(groups []FlowGroup, ctx common.FlushContext) int {
		// Collect active reporters for sequence tracking and exporter metadata.
		// GhostReporters are carry-over metadata snapshots and should not affect
		// sequence delta calculations or exporter registration.
		var activeFlows []*common.Flow
		for _, group := range groups {
			activeFlows = append(activeFlows, group.Reporters...)
		}

		agg.logger.Debugf("Flushing %d flow groups to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)",
			len(groups), time.Since(ctx.FlushTime).Milliseconds(), agg.flowAcc.GetFlowContextCount())

		agg.emitSequenceMetrics(agg.getSequenceDelta(activeFlows))

		if len(groups) > 0 {
			agg.sendMergedFlows(groups, ctx.FlushTime)
		}
		agg.sendExporterMetadata(activeFlows, ctx.FlushTime)
		return len(groups)
	}

	return agg
}

// NewFlowAggregator returns a FlowAggregatorRunner, selecting the standard or dedup
// implementation based on the config. External consumers that don't need the type
// parameter should use this constructor.
func NewFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) FlowAggregatorRunner {
	if conf.DeduplicationEnabled {
		return NewDedupFlowAggregator(snd, epForwarder, conf, hostname, logger, rdnsQuerier)
	}
	return NewStandardFlowAggregator(snd, epForwarder, conf, hostname, logger, rdnsQuerier)
}

// Start will start the FlowAggregator worker
func (agg *FlowAggregator[T]) Start() {
	agg.logger.Info("Flow Aggregator started")
	go agg.run()
	agg.flushLoop() // blocking call
}

// Stop will stop running FlowAggregator
func (agg *FlowAggregator[T]) Stop() {
	close(agg.stopChan)
	<-agg.flushLoopDone
	<-agg.runDone
}

// GetFlowInChan returns flow input chan
func (agg *FlowAggregator[T]) GetFlowInChan() chan *common.Flow {
	return agg.flowIn
}

func (agg *FlowAggregator[T]) run() {
	for {
		select {
		case <-agg.stopChan:
			agg.logger.Info("Stopping aggregator")
			agg.runDone <- struct{}{}
			return
		case flow := <-agg.flowIn:
			agg.receivedFlowCount.Inc()
			agg.flowAcc.Add(flow)
		}
	}
}

func (agg *FlowAggregator[T]) sendFlows(flows []*common.Flow, flushTime time.Time) {
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
		err = agg.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesNetFlow)
		if err != nil {
			// at the moment, SendEventPlatformEventBlocking can only fail if the event type is invalid
			agg.logger.Errorf("Error sending to event platform forwarder: %s", err)
			continue
		}
	}
}

// sendMergedFlows sends one merged event per 5-tuple group. All reporters (active +
// ghost) are included so the platform has the full picture for flow_role assignment.
func (agg *FlowAggregator[T]) sendMergedFlows(groups []FlowGroup, flushTime time.Time) {
	for _, group := range groups {
		if len(group.Reporters) == 0 {
			continue
		}
		reporters := append(group.Reporters, group.GhostReporters...)
		mergedPayload := buildMergedPayload(reporters, agg.hostname, flushTime)
		payloadBytes, err := json.Marshal(mergedPayload)
		if err != nil {
			agg.logger.Errorf("Error marshalling merged flow payload: %s", err)
			continue
		}
		agg.logger.Tracef("flushed merged flow: %s", string(payloadBytes))

		m := message.NewMessage(payloadBytes, nil, "", 0)
		err = agg.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesNetFlow)
		if err != nil {
			agg.logger.Errorf("Error sending to event platform forwarder: %s", err)
			continue
		}
	}
}

func (agg *FlowAggregator[T]) sendExporterMetadata(flows []*common.Flow, flushTime time.Time) {
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
			err = agg.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata)
			if err != nil {
				agg.logger.Errorf("Error sending event platform event for netflow exporter metadata: %s", err)
			}
		}
	}
}

func (agg *FlowAggregator[T]) flushLoop() {
	flushFlowsToSendTicker := agg.NewTicker(agg.FlushConfig.FlushTickFrequency)
	if flushFlowsToSendTicker == nil {
		agg.logger.Debug("flushFlowsToSendInterval set to 0: will never flush automatically")
	}

	rollupTrackersRefresh := agg.NewTicker(agg.rollupTrackerRefreshInterval)
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

			// Calculate how many flushes should have happened since the last tick. Ticks can be missed for various reasons,
			// including CPU pauses or if this goroutine gets blocked longer than usual waiting on network requests.
			var expectedFlushes int64 = 1
			if !lastFlushTime.IsZero() {
				// add one millisecond to account for small variations in the time.Ticker
				timeSinceLast := flushStartTime.Sub(lastFlushTime) + time.Millisecond

				// We do not want this to default to 0 for small time deltas or if time.Tick fires a little early.
				// Make the expected flushes either 1 or the result of calculation
				expectedFlushes = max(1, int64(timeSinceLast/agg.FlushConfig.FlushTickFrequency))
			}
			flushCtx := common.FlushContext{
				FlushTime:     flushStartTime,
				LastFlushedAt: lastFlushTime,
				NumFlushes:    expectedFlushes,
			}

			lastFlushTime = flushStartTime
			agg.flush(flushCtx)
			agg.sender.Gauge("datadog.netflow.aggregator.flush_duration", time.Since(flushStartTime).Seconds(), "", nil)
			agg.sender.Commit()
		// refresh rollup trackers
		case <-rollupTrackersRefresh:
			agg.rollupTrackersRefresh()
		}
	}
}

// flush flushes the accumulator and submits the result via the type-specific submit function.
func (agg *FlowAggregator[T]) flush(ctx common.FlushContext) int {
	flowsContexts := agg.flowAcc.GetFlowContextCount()
	result := agg.flowAcc.Flush(ctx)
	flushCount := agg.submit(result, ctx)

	agg.emitFlushMetrics(flowsContexts, flushCount)
	agg.flushedFlowCount.Add(uint64(flushCount))
	return flushCount
}

func (agg *FlowAggregator[T]) emitFlushMetrics(flowsContexts int, flushCount int) {
	agg.sender.MonotonicCount("datadog.netflow.aggregator.hash_collisions", float64(agg.flowAcc.HashCollisionCount().Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.Count("datadog.netflow.aggregator.flows_flushed", float64(flushCount), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.flows_contexts", float64(flowsContexts), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_size", float64(agg.flowAcc.PortRollup().GetCurrentStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_size", float64(agg.flowAcc.PortRollup().GetNewStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.capacity", float64(cap(agg.flowIn)), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.length", float64(len(agg.flowIn)), "", nil)

	err := agg.submitCollectorMetrics()
	if err != nil {
		agg.logger.Warnf("error submitting collector metrics: %s", err)
	}
}

// emitSequenceMetrics reports sequence delta metrics for each exporter.
func (agg *FlowAggregator[T]) emitSequenceMetrics(sequenceDeltaPerExporter map[sequenceDeltaKey]sequenceDeltaValue) {
	for key, seqDelta := range sequenceDeltaPerExporter {
		tags := []string{"device_namespace:" + key.Namespace, "exporter_ip:" + key.ExporterIP, "flow_type:" + string(key.FlowType)}
		agg.sender.Count("datadog.netflow.aggregator.sequence.delta", float64(seqDelta.Delta), "", tags)
		agg.sender.Gauge("datadog.netflow.aggregator.sequence.last", float64(seqDelta.LastSequence), "", tags)
		if seqDelta.Reset {
			agg.sender.Count("datadog.netflow.aggregator.sequence.reset", float64(1), "", tags)
		}
	}
}

// getSequenceDelta return the delta of current sequence number compared to previously saved sequence number
// Since we track per exporterIP, the returned delta is only accurate when for the specific exporterIP there is
// only one NetFlow9/IPFIX observation domain, NetFlow5 engineType/engineId, sFlow agent/subagent.
func (agg *FlowAggregator[T]) getSequenceDelta(flowsToFlush []*common.Flow) map[sequenceDeltaKey]sequenceDeltaValue {
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

func (agg *FlowAggregator[T]) rollupTrackersRefresh() {
	agg.logger.Debugf("Rollup tracker refresh: use new store as current store")
	agg.flowAcc.PortRollup().UseNewStoreAsCurrentStore()
}

func (agg *FlowAggregator[T]) submitCollectorMetrics() error {
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
