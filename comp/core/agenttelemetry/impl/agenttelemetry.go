// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetryimpl provides the implementation of the agenttelemetry component.
package agenttelemetryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/atomic"
)

type atel struct {
	cfgComp config.Component
	logComp log.Component
	telComp telemetry.Component

	enabled      bool
	localEmitter string
	sender       sender
	runner       runner
	atelCfg      *Config

	lightTracer *installertelemetry.Telemetry

	cancelCtx context.Context
	cancel    context.CancelFunc

	startupSpan *installertelemetry.Span

	prevPromMetricCounterValues   map[string]float64
	prevPromMetricHistogramValues map[string]uint64
	prevPromMetricValuesMU        sync.Mutex

	// Errortracking: bounded channel drained by a runner-scheduled job.
	// SubmitErrorLog enqueues here non-blockingly; flushErrortracking
	// drains the channel on each tick and on shutdown, dispatching via
	// sender.sendLogsBatch.
	//
	// errortrackingEnabled gates allocation of errLogsCh and registration
	// of the flush job on the composed gate (IsAgentTelemetryEnabled &&
	// agent_telemetry.errortracking.enabled), so deployments that don't
	// opt in pay zero overhead (no buffer, no scheduled job).
	errortrackingEnabled bool
	errLogsCh            chan errortracking.ErrorLog
	errLogsDropped       *atomic.Uint64
	errLogsFlushInterval time.Duration
	errLogsStartupJitter time.Duration
	shutdownDrainTimeout time.Duration
}

const (
	defaultErrortrackingFlushIntervalSeconds     = 60
	defaultErrortrackingBufferSize               = 2048
	defaultErrortrackingStartupJitterSeconds     = 0
	defaultErrortrackingShutdownDrainTimeoutSecs = 5
)

// Requires defines the dependencies for the agenttelemetry component
type Requires struct {
	compdef.In

	Log       log.Component
	Config    config.Component
	Telemetry telemetry.Component

	Lc compdef.Lifecycle
}

// Provides defines the output of the agenttelemetry component
type Provides struct {
	compdef.Out

	Comp     agenttelemetry.Component
	Endpoint api.AgentEndpointProvider
}

// Interfacing with runner.
//
// A single job type drives both the periodic metric-profile flush and
// the errortracking flush. The profiles slice doubles as a
// discriminator: a nil profiles slice means "this is the errortracking
// flush job"; a non-nil slice means "this is a metric-profile tick".
// Threading both behaviours through the same job avoids widening the
// runner's interface for a one-off second consumer.
type job struct {
	a        *atel
	profiles []*Profile
	schedule Schedule
}

func (j job) Run() {
	if j.profiles == nil {
		// Errortracking runner tick: use the lifecycle context so an
		// in-flight POST cancels promptly when the agent stops. The
		// final shutdown drain in atel.stop uses a fresh background
		// context with a bounded timeout instead.
		j.a.flushErrortracking(j.a.cancelCtx)
		return
	}
	j.a.run(j.profiles)
}

// Passing metrics to sender Interfacing with sender
type agentmetric struct {
	name    string
	metrics []*dto.Metric
	family  *dto.MetricFamily
}

func createSender(
	cfgComp config.Component,
	logComp log.Component,
) (sender, error) {
	client := newSenderClientImpl(cfgComp)
	sender, err := newSenderImpl(cfgComp, logComp, client)
	if err != nil {
		logComp.Errorf("Failed to create agent telemetry sender: %s", err.Error())
	}
	return sender, err
}

func localEmitterFromFlavor(agentFlavor string) string {
	return strings.ReplaceAll(agentFlavor, "_", "-")
}

func createAtel(
	cfgComp config.Component,
	logComp log.Component,
	telComp telemetry.Component,
	sender sender,
	runner runner,
	localEmitter string) *atel {
	// Parse Agent Telemetry Configuration configuration
	atelCfg, err := parseConfig(cfgComp)
	if err != nil {
		logComp.Errorf("Failed to parse agent telemetry config: %s", err.Error())
		return &atel{}
	}
	if !atelCfg.Enabled {
		logComp.Info("Agent telemetry is disabled")
		return &atel{}
	}

	if sender == nil {
		sender, err = createSender(cfgComp, logComp)
		if err != nil {
			logComp.Errorf("Failed to create agent telemetry sender: %s", err.Error())
			return &atel{}
		}
	}

	if runner == nil {
		runner = newRunnerImpl()
	}

	installertelemetry.SetSamplingRate("agent.startup", atelCfg.StartupTraceSampling)

	tracerHTTPClient := &http.Client{
		Transport: httputils.CreateHTTPTransport(cfgComp),
	}

	// Only allocate the errortracking channel (and later spawn the flush
	// goroutine in start) when the errortracking feature is enabled.
	// Otherwise leave errLogsCh nil; SubmitErrorLog is then a no-op
	// (see the errLogsCh==nil guard there) and we avoid the buffer +
	// idle goroutine for deployments that don't opt in.
	//
	// Gate composes with IsAgentTelemetryEnabled so gov/FIPS sites
	// (parent agent_telemetry is excluded for them) automatically opt
	// out without needing a separate exclusion list.
	errortrackingEnabled := utils.IsErrorTrackingEnabled(cfgComp)

	bufferSize := getNonNegativeErrortrackingInt(
		cfgComp,
		logComp,
		"agent_telemetry.errortracking.buffer_size",
		defaultErrortrackingBufferSize,
	)
	flushIntervalSeconds := getNonNegativeErrortrackingInt(
		cfgComp,
		logComp,
		"agent_telemetry.errortracking.flush_interval_seconds",
		defaultErrortrackingFlushIntervalSeconds,
	)
	startupJitterSeconds := getNonNegativeErrortrackingInt(
		cfgComp,
		logComp,
		"agent_telemetry.errortracking.startup_jitter_seconds",
		defaultErrortrackingStartupJitterSeconds,
	)

	var errLogsCh chan errortracking.ErrorLog
	if errortrackingEnabled {
		errLogsCh = make(chan errortracking.ErrorLog, bufferSize)
	}

	// Final-drain context budget — bounded so a hung intake cannot
	// block agent shutdown. See atel.stop for usage.
	shutdownDrainTimeoutSeconds := getNonNegativeErrortrackingInt(
		cfgComp,
		logComp,
		"agent_telemetry.errortracking.shutdown_drain_timeout_seconds",
		defaultErrortrackingShutdownDrainTimeoutSecs,
	)

	return &atel{
		enabled:      true,
		localEmitter: localEmitter,
		cfgComp:      cfgComp,
		logComp:      logComp,
		telComp:      telComp,
		sender:       sender,
		runner:       runner,
		atelCfg:      atelCfg,

		lightTracer: installertelemetry.NewTelemetry(
			tracerHTTPClient,
			utils.SanitizeAPIKey(cfgComp.GetString("api_key")),
			cfgComp.GetString("site"),
			"datadog-agent",
		),

		prevPromMetricCounterValues:   make(map[string]float64),
		prevPromMetricHistogramValues: make(map[string]uint64),

		errortrackingEnabled: errortrackingEnabled,
		errLogsCh:            errLogsCh,
		errLogsDropped:       atomic.NewUint64(0),
		errLogsFlushInterval: time.Duration(flushIntervalSeconds) * time.Second,
		errLogsStartupJitter: time.Duration(startupJitterSeconds) * time.Second,
		shutdownDrainTimeout: time.Duration(shutdownDrainTimeoutSeconds) * time.Second,
	}
}

func getNonNegativeErrortrackingInt(cfgComp config.Component, logComp log.Component, key string, defaultValue int) int {
	value := cfgComp.GetInt(key)
	if value >= 0 {
		return value
	}

	logComp.Warnf("%s is negative (%d); using default %d", key, value, defaultValue)
	return defaultValue
}

// NewComponent creates a new agent telemetry component.
func NewComponent(deps Requires) Provides {
	a := createAtel(
		deps.Config,
		deps.Log,
		deps.Telemetry,
		nil,
		nil,
		localEmitterFromFlavor(flavor.GetFlavor()),
	)

	// If agent telemetry is enabled and configured properly add the start and stop hooks
	if a.enabled {
		deps.Lc.Append(compdef.Hook{
			OnStart: func(_ context.Context) error {
				return a.start()
			},
			OnStop: func(_ context.Context) error {
				return a.stop()
			},
		})
	}

	return Provides{
		Comp:     a,
		Endpoint: api.NewAgentEndpointProvider(a.writePayload, "/metadata/agent-telemetry", "GET"),
	}
}

type metricAggregationKey struct {
	emitter       string
	preservedTags string
}

// Prometheus preserves ':' in label values, so delimiter-only concatenation
// can encode distinct sorted label sets identically. Length prefixes make each
// name/value boundary unambiguous.
func encodeSortedAggregationLabels(labels []*dto.LabelPair) string {
	var key strings.Builder
	for _, label := range labels {
		name := label.GetName()
		value := label.GetValue()
		key.WriteString(strconv.Itoa(len(name)))
		key.WriteByte(':')
		key.WriteString(name)
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

func effectiveEmitter(labels []*dto.LabelPair, localEmitter string) string {
	for _, label := range labels {
		if label.GetName() == "emitter" && label.GetValue() != "" {
			return label.GetValue()
		}
	}

	if localEmitter != "" {
		return localEmitter
	}
	return "agent"
}

func (a *atel) aggregateMetricTags(mCfg *MetricConfig, mt dto.MetricType, ms []*dto.Metric) []*dto.Metric {
	if len(ms) == 0 {
		return nil
	}

	aggregates := make(map[metricAggregationKey]*dto.Metric)
	totalMetrics := make(map[string]*dto.Metric)
	totalCounts := make(map[string]int)

	for _, metric := range ms {
		origLabels := metric.GetLabel()
		emitter := effectiveEmitter(origLabels, a.localEmitter)
		preservedLabels := make([]*dto.LabelPair, 0, len(origLabels))
		for _, label := range origLabels {
			if label.GetName() == "emitter" {
				continue
			}
			if _, ok := mCfg.preserveTagsMap[label.GetName()]; ok {
				preservedLabels = append(preservedLabels, label)
			}
		}
		preservedLabels = cloneLabelsSorted(preservedLabels)

		key := metricAggregationKey{
			emitter:       emitter,
			preservedTags: encodeSortedAggregationLabels(preservedLabels),
		}
		if aggregate, ok := aggregates[key]; ok {
			aggregateMetric(mt, aggregate, metric)
		} else {
			aggregate := &dto.Metric{}
			aggregateMetric(mt, aggregate, metric)
			emitterName := "emitter"
			aggregate.Label = append(preservedLabels, &dto.LabelPair{Name: &emitterName, Value: &emitter})
			aggregate.Label = cloneLabelsSorted(aggregate.Label)
			aggregates[key] = aggregate
		}

		if mCfg.AggregateTotal {
			totalMetric, ok := totalMetrics[emitter]
			if !ok {
				totalMetric = &dto.Metric{}
				totalMetrics[emitter] = totalMetric
			}
			aggregateMetric(mt, totalMetric, metric)
			totalCounts[emitter]++
		}
	}

	results := slices.Collect(maps.Values(aggregates))
	for _, emitter := range slices.Sorted(maps.Keys(totalMetrics)) {
		emitterName := "emitter"
		totalName := "total"
		totalValue := strconv.Itoa(totalCounts[emitter])
		totalMetric := totalMetrics[emitter]
		totalMetric.Label = []*dto.LabelPair{
			{Name: &emitterName, Value: &emitter},
			{Name: &totalName, Value: &totalValue},
		}
		results = append(results, totalMetric)
	}

	return results
}

// Using Prometheus  terminology. Metrics name or in "Prom" MetricFamily is technically a Datadog metrics.
// dto.Metric are a metric values for each timeseries (tag/value combination).
func buildKeysForMetricsPreviousValues(mt dto.MetricType, metricName string, metrics []*dto.Metric) []string {
	keyNames := make([]string, 0, len(metrics))
	for _, m := range metrics {
		var keyName string
		tags := m.GetLabel()
		if len(tags) == 0 {
			// A source-label-free MetricFamily has one time series, so its metric name is a stable
			// previous-value key. The mandatory emitter label is attached during later aggregation.
			keyName = metricName
		} else {
			// A source-labeled MetricFamily has one metric per time series. Each time series needs a
			// unique, stable previous-value key formed from its source label names and values.
			keyName = fmt.Sprintf("%s%s:", metricName, convertLabelsToKey(tags))
		}

		if mt == dto.MetricType_HISTOGRAM {
			// For each source time series, track every explicit histogram bucket plus the implicit
			// "+Inf" bucket. For example, 3 time series with 4 buckets require 15 keys (3x(4+1)).
			for _, bucket := range m.Histogram.GetBucket() {
				keyNames = append(keyNames, fmt.Sprintf("%v:%v", keyName, bucket.GetUpperBound()))
			}
		}

		// Add the key for Counter, Gauge metric and HISTOGRAM's +Inf bucket
		keyNames = append(keyNames, keyName)
	}

	return keyNames
}

// Swap current value with the previous value and deduct the previous value from the current value
func deductAndUpdatePrevValue(key string, prevPromMetricValues map[string]uint64, curValue *uint64) {
	origCurValue := *curValue
	if prevValue, ok := prevPromMetricValues[key]; ok {
		*curValue -= prevValue
	}
	prevPromMetricValues[key] = origCurValue
}

func convertPromHistogramsToDatadogHistogramsValues(metrics []*dto.Metric, prevPromMetricValues map[string]uint64, keyNames []string) {
	if len(metrics) > 0 {
		bucketCount := len(metrics[0].Histogram.GetBucket())
		var prevValue uint64

		for i, m := range metrics {
			// 1. deduct the previous cumulative count from each explicit  buckets
			for j, b := range m.Histogram.GetBucket() {
				deductAndUpdatePrevValue(keyNames[(i*(bucketCount+1))+j], prevPromMetricValues, b.CumulativeCount)
			}
			// 2. deduct the previous cumulative count from the implicit  "+Inf" bucket
			deductAndUpdatePrevValue(keyNames[((i+1)*(bucketCount+1))-1], prevPromMetricValues, m.Histogram.SampleCount)

			// 3. "De-cumulate" next explicit bucket value from the preceding bucket value
			prevValue = 0
			for _, b := range m.Histogram.GetBucket() {
				curValue := b.GetCumulativeCount()
				*b.CumulativeCount -= prevValue
				prevValue = curValue
			}
			// 4. "De-cumulate" implicit "+Inf" bucket value from the preceding bucket value
			*m.Histogram.SampleCount -= prevValue
		}
	}
}

func convertPromCountersToDatadogCountersValues(metrics []*dto.Metric, prevPromMetricValues map[string]float64, keyNames []string) {
	for i, m := range metrics {
		key := keyNames[i]
		curValue := m.GetCounter().GetValue()

		// Adjust the counter value if found
		if prevValue, ok := prevPromMetricValues[key]; ok {
			*m.GetCounter().Value -= prevValue
		}

		// Upsert the cache of previous counter values
		prevPromMetricValues[key] = curValue
	}
}

// Convert ...
//  1. Prom Counters from monotonic to non-monotonic by resetting the counter during this call
//  2. Prom Histograms buckets counters from monotonic to non-monotonic by resetting the counter during this call
func (a *atel) convertPromMetricToDatadogMetricsValues(mt dto.MetricType, metricName string, metrics []*dto.Metric) {
	if len(metrics) > 0 && (mt == dto.MetricType_COUNTER || mt == dto.MetricType_HISTOGRAM) {
		// Build the keys for the metrics (or buckets) to cache their previous values
		keyNames := buildKeysForMetricsPreviousValues(mt, metricName, metrics)

		a.prevPromMetricValuesMU.Lock()
		defer a.prevPromMetricValuesMU.Unlock()
		if mt == dto.MetricType_HISTOGRAM {
			convertPromHistogramsToDatadogHistogramsValues(metrics, a.prevPromMetricHistogramValues, keyNames)
		} else {
			convertPromCountersToDatadogCountersValues(metrics, a.prevPromMetricCounterValues, keyNames)
		}
	}
}

func isMetricFiltered(p *Profile, mCfg *MetricConfig, mt dto.MetricType, m *dto.Metric) bool {
	// filter out zero values if specified in the profile
	if p.excludeZeroMetric && isZeroValueMetric(mt, m) {
		return false
	}

	// filter out if contains excluded tags
	if len(p.excludeTagsMap) > 0 && areTagsMatching(m.GetLabel(), p.excludeTagsMap) {
		return false
	}

	if !mCfg.preserveTagsExists {
		return true
	}

	hasNonEmitterPreserveTag := false
	for tagName := range mCfg.preserveTagsMap {
		if tagName != "emitter" {
			hasNonEmitterPreserveTag = true
			break
		}
	}
	if !hasNonEmitterPreserveTag {
		return true
	}

	labels := m.GetLabel()
	for i, label := range labels {
		if label.GetName() != "emitter" && areTagsMatching(labels[i:i+1], mCfg.preserveTagsMap) {
			return true
		}
	}
	return false
}

func (a *atel) transformMetricFamily(p *Profile, mfam *dto.MetricFamily) *agentmetric {
	var mCfg *MetricConfig
	var ok bool

	// Check if the metric is included in the profile. Normalize "__" to "_"
	// so that metrics registered with or without NoDoubleUnderscoreSep are matched.
	normalizedName := strings.Replace(mfam.GetName(), "__", "_", 1)
	if mCfg, ok = p.metricsMap[normalizedName]; !ok {
		return nil
	}

	// Filter out not supported types
	mt := mfam.GetType()
	if !isSupportedMetricType(mt) {
		return nil
	}

	// Filter source time series according to zero-value, excluded-label, and preserve-label rules.
	var fm []*dto.Metric
	for _, m := range mfam.Metric {
		if isMetricFiltered(p, mCfg, mt, m) {
			fm = append(fm, m)
		}
	}

	// nothing to report
	if len(fm) == 0 {
		return nil
	}

	// Convert Prom Metrics values to the corresponding Datadog metrics style values.
	// This must happen BEFORE aggregation so that delta cache keys are based on raw
	// Prometheus labels (which are stable), not on synthetic labels like "total" whose
	// value encodes the timeseries count and changes when timeseries appear/disappear.
	// Mathematically: sum(deltas) == delta(sums), so aggregating deltas is equivalent.
	a.convertPromMetricToDatadogMetricsValues(mt, mCfg.Name, fm)

	// Aggregate the metric tags (now operating on deltas rather than cumulative values)
	amt := a.aggregateMetricTags(mCfg, mt, fm)

	return &agentmetric{
		name:    mCfg.Name,
		metrics: amt,
		family:  mfam,
	}
}

// coalesceMetricFamilies merges compatible metric families with the same name.
//
// The regular and default telemetry registries are gathered separately. Coalescing lets profile aggregation see all
// time series together instead of later payload writes overwriting earlier ones in the sender's metric map.
func coalesceMetricFamilies(pms []*telemetry.MetricFamily) []*telemetry.MetricFamily {
	mergedByName := make(map[string]*telemetry.MetricFamily, len(pms))
	merged := make([]*telemetry.MetricFamily, 0, len(pms))

	for _, pm := range pms {
		if pm == nil || pm.Name == nil || pm.Type == nil {
			merged = append(merged, pm)
			continue
		}

		name := pm.GetName()
		existing := mergedByName[name]
		if existing == nil {
			mergedByName[name] = pm
			merged = append(merged, pm)
			continue
		}
		if existing.GetType() != pm.GetType() {
			merged = append(merged, pm)
			continue
		}

		existing.Metric = append(existing.Metric, pm.Metric...)
	}

	return merged
}

func (a *atel) reportAgentMetrics(session *senderSession, pms []*telemetry.MetricFamily, p *Profile) {
	// If no metrics are configured nothing to report
	if len(p.metricsMap) == 0 {
		return
	}

	a.logComp.Debugf("Collect Agent Metric telemetry for profile %s", p.Name)

	// ... and filter them according to the profile configuration
	var metrics []*agentmetric
	for _, pm := range pms {
		if am := a.transformMetricFamily(p, pm); am != nil {
			metrics = append(metrics, am)
		}
	}

	// Send the metrics if any were filtered
	if len(metrics) == 0 {
		a.logComp.Debug("No Agent Metric telemetry collected")
		return
	}

	// Send the metrics if any were filtered
	a.logComp.Debugf("Reporting Agent Metric telemetry for profile %s", p.Name)

	a.sender.sendAgentMetricPayloads(session, metrics)
}

func (a *atel) loadPayloads(profiles []*Profile) (*senderSession, error) {
	// Gather all prom metrics. Currently Gather() does not allow filtering by
	// metric name, so we need to gather all metrics and filter them on our own.
	pms, err := a.telComp.Gather(false)
	if err != nil {
		a.logComp.Errorf("failed to get filtered telemetry metrics: %v", err)
		return nil, err
	}

	// Ensure that metrics from the default Prometheus registry are also collected.
	pmsDefault, errDefault := a.telComp.Gather(true)
	if errDefault == nil {
		pms = append(pms, pmsDefault...)
	} else {
		// Not a fatal error, just log it
		a.logComp.Errorf("failed to get filtered telemetry metrics: %v", err)
	}

	pms = coalesceMetricFamilies(pms)

	// All metrics stored in the "pms" slice above must follow the format:
	//    <subsystem>__<metric_name>
	// The "subsystem" and "name" should be concatenated with a double underscore ("__") separator,
	// e.g., "checks__execution_time". Therefore, the "Options.NoDoubleUnderscoreSep: true" option
	// must not be used when creating metrics.

	session := a.sender.startSession(a.cancelCtx)
	for _, p := range profiles {
		a.reportAgentMetrics(session, pms, p)
	}
	return session, nil
}

// run runs the agent telemetry for a given profile. It is triggered by the runner
// according to the profiles schedule.
func (a *atel) run(profiles []*Profile) {
	a.logComp.Info("Starting agent telemetry run")
	session, err := a.loadPayloads(profiles)
	if err != nil {
		a.logComp.Errorf("failed to load agent telemetry session: %s", err)
		return
	}

	err = a.sender.flushSession(session)
	if err != nil {
		a.logComp.Errorf("failed to flush agent telemetry session: %s", err)
		return
	}
}

func (a *atel) writePayload(w http.ResponseWriter, _ *http.Request) {
	if !a.enabled {
		httputils.SetJSONError(w, errors.New("agent-telemetry is not enabled. See https://docs.datadoghq.com/data_security/agent/?tab=datadogyaml#telemetry-collection for more info"), 400)
		return
	}

	a.logComp.Info("Showing agent telemetry payload")
	payload, err := a.getAsJSON()
	if err != nil {
		httputils.SetJSONError(w, a.logComp.Error(err.Error()), 500)
		return
	}

	w.Write(payload)
}

func (a *atel) getAsJSON() ([]byte, error) {
	session, err := a.loadPayloads(a.atelCfg.Profiles)
	if err != nil {
		return nil, fmt.Errorf("unable to load agent telemetry payload: %w", err)
	}
	payload := session.flush()

	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("unable to marshal agent telemetry payload: %w", err)
	}

	jsonPayloadScrubbed, err := scrubber.ScrubJSON(jsonPayload)
	if err != nil {
		return nil, fmt.Errorf("unable to scrub agent telemetry payload: %w", err)
	}

	var prettyPayload bytes.Buffer
	err = json.Indent(&prettyPayload, jsonPayloadScrubbed, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("unable to pretified agent telemetry payload: %w", err)
	}

	return prettyPayload.Bytes(), nil
}

func (a *atel) SendEvent(eventType string, eventPayload []byte) error {
	// Check if the telemetry is enabled
	if !a.enabled {
		return errors.New("agent telemetry is not enabled")
	}

	// Check if the payload type is registered
	eventInfo, ok := a.atelCfg.events[eventType]
	if !ok {
		a.logComp.Errorf("Payload type `%s` has to be registered to be sent", eventType)
		return fmt.Errorf("Payload type `%s` is not registered", eventType)
	}

	// Convert payload to JSON
	var eventPayloadJSON map[string]interface{}
	err := json.Unmarshal(eventPayload, &eventPayloadJSON)
	if err != nil {
		a.logComp.Errorf("Failed to unmarshal payload: %s", err)
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Send the payload
	ss := a.sender.startSession(a.cancelCtx)
	a.sender.sendEventPayload(ss, eventInfo, eventPayloadJSON)
	err = a.sender.flushSession(ss)
	if err != nil {
		a.logComp.Errorf("failed to flush sent payload: %v", err)
		return err
	}

	return nil
}

// SubmitErrorLog is the per-log entry point. Non-blocking: enqueues
// into the bounded errLogsCh buffer; on overflow, drops silently and
// increments errLogsDropped (the calling goroutine — the slog handler
// hot path — MUST NOT block on a misbehaving backend).
//
// Recursion prevention: the flush path (sendLogsBatch → sendPayload)
// is required to log only at Debug. A future addition of any Errorf in
// that path would re-enter this method via the slog handler.
func (a *atel) SubmitErrorLog(log errortracking.ErrorLog) {
	if !a.enabled {
		return
	}
	if a.errLogsCh == nil {
		// errortracking feature flag disabled: no channel allocated and
		// no flush goroutine running. Calling this should be a no-op
		// rather than a nil-channel send (which would block forever).
		return
	}
	select {
	case a.errLogsCh <- log:
	default:
		a.errLogsDropped.Add(1)
	}
}

// flushErrortracking drains logs currently buffered in errLogsCh and
// dispatches them as ONE HTTP call via sender.sendLogsBatch.
// The drain is bounded to len(errLogsCh) at flush start: items that
// arrive after the snapshot are left for the next tick, preventing a
// hot error stream from holding the flush job indefinitely.
// A no-op when errortracking is disabled (errLogsCh == nil) or the
// channel is empty.
//
// Called from two places:
//  1. The runner-scheduled job tick (via job.Run when profiles==nil),
//     using a.cancelCtx so an in-flight POST cancels promptly on
//     agent shutdown.
//  2. The final drain in atel.stop, using a fresh background-derived
//     context with a.shutdownDrainTimeout so the post-stop POSTs do
//     NOT inherit the already-canceled lifecycle context — without
//     this the shutdown drain would silently drop every buffered
//     log (HTTP returns context canceled immediately).
//
// Behavioral note: send failures (5xx, network) are logged at Debug
// and the batch is dropped. The pkg/util/log/errortracking handler is
// a producer with no retry expectation; retrying at flush time would
// block subsequent ticks and require additional buffering complexity.
func (a *atel) flushErrortracking(ctx context.Context) {
	if a.errLogsCh == nil {
		return
	}
	// Snapshot the current queue depth so a hot producer cannot extend
	// this batch indefinitely: items arriving after the snapshot wait for
	// the next tick rather than growing errLogs beyond cap(errLogsCh).
	n := len(a.errLogsCh)
	if n == 0 {
		return
	}
	errLogs := make([]errortracking.ErrorLog, 0, n)
	for range n {
		select {
		case l := <-a.errLogsCh:
			errLogs = append(errLogs, l)
		default:
			// Channel drained before snapshot count — harmless, stop early.
			break
		}
	}
	if len(errLogs) == 0 {
		return
	}
	logs := make([]Log, len(errLogs))
	for i, l := range errLogs {
		logs[i] = enrichErrorLog(l)
	}
	if err := a.sender.sendLogsBatch(ctx, logs); err != nil {
		a.logComp.Debugf("errortracking flush failed (%d logs): %v", len(errLogs), err)
	}
}

func (a *atel) StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context) {
	if a.lightTracer != nil {
		return installertelemetry.StartSpanFromContext(a.cancelCtx, operationName)
	}
	return &installertelemetry.Span{}, a.cancelCtx
}

// start is called by FX when the application starts.
func (a *atel) start() error {
	a.logComp.Infof("Starting agent telemetry for %d schedules and %d profiles", len(a.atelCfg.schedule), len(a.atelCfg.Profiles))

	a.cancelCtx, a.cancel = context.WithCancel(context.Background())

	if a.lightTracer != nil {
		// Start internal telemetry trace
		a.startupSpan, a.cancelCtx = installertelemetry.StartSpanFromContext(a.cancelCtx, "agent.startup")
		go func() {
			timing := time.After(1 * time.Minute)
			select {
			case <-a.cancelCtx.Done():
				if a.startupSpan != nil {
					a.startupSpan.Finish(a.cancelCtx.Err())
				}
			case <-timing:
				if a.startupSpan != nil {
					a.startupSpan.Finish(nil)
				}
			}
		}()
	}

	// Start the runner and add the jobs.
	a.runner.start()
	for sh, pp := range a.atelCfg.schedule {
		a.runner.addJob(job{
			a:        a,
			profiles: pp,
			schedule: sh,
		})
	}

	// Register the errortracking flush as a runner job when the feature
	// is enabled. The runner's cron-based scheduling replaces the
	// previous custom ticker + WaitGroup goroutine; in-flight cron jobs
	// are awaited via runner.stop() in atel.stop. profiles==nil is the
	// discriminator the job's Run dispatches on.
	if a.errortrackingEnabled {
		flushPeriodSec := uint(a.errLogsFlushInterval / time.Second)
		if flushPeriodSec == 0 {
			// Guard against a zero flush interval from misconfiguration; Period:0
			// would schedule a degenerate job. Default is 60 s; floor at 5 s.
			a.logComp.Warnf("agent_telemetry.errortracking.flush_interval_seconds resolved to 0; clamping to 5 s")
			flushPeriodSec = 5
		}
		var startAfterSec uint
		if a.errLogsStartupJitter > 0 {
			// rand.Int63n panics on n<=0; the > 0 guard above protects.
			// The configured value is the MAX random jitter; the actual
			// per-startup delay is a uniform pick in [0, max).
			startAfterSec = uint(time.Duration(rand.Int63n(int64(a.errLogsStartupJitter))) / time.Second)
		}
		a.runner.addJob(job{
			a: a,
			schedule: Schedule{
				Period:     flushPeriodSec,
				Iterations: 0,
				StartAfter: startAfterSec,
			},
		})
	}

	return nil
}

// stop is called by FX when the application stops.
//
// Shutdown ordering for the errortracking path (records-after-drain
// safety):
//  1. Clear the submitter + bouncer slots so producers stop reaching
//     SubmitErrorLog. After this point Handler.Enabled is false and
//     the parent multi-handler short-circuits.
//  2. Cancel the lifecycle context (a.cancel) — any in-flight runner-
//     scheduled flush tick promptly cancels its HTTP POST.
//  3. runner.stop() blocks new ticks; its returned Done channel
//     signals when in-flight cron jobs have finished, taking the place
//     of the previous custom WaitGroup-based barrier.
//  4. Final drain: flushErrortracking with a fresh background-derived
//     context + shutdownDrainTimeout, so records buffered between the
//     last tick and the slot-clear are still sent.
func (a *atel) stop() error {
	pkglogsetup.RegisterErrortrackingSubmitter(nil)
	pkglogsetup.RegisterErrortrackingBouncer(nil)

	if a.startupSpan != nil {
		a.startupSpan.Finish(nil)
	}

	a.logComp.Info("Stopping agent telemetry")
	a.cancel()

	if a.lightTracer != nil {
		a.lightTracer.Stop()
	}

	runnerCtx := a.runner.stop()
	<-runnerCtx.Done()

	// Final errortracking drain. Uses a fresh background-derived ctx
	// because a.cancelCtx is already done; the bounded timeout caps the
	// budget so a hung intake cannot block shutdown.
	if a.errortrackingEnabled {
		shutdownCtx, cancelDrain := context.WithTimeout(context.Background(), a.shutdownDrainTimeout)
		a.flushErrortracking(shutdownCtx)
		cancelDrain()
	}

	a.logComp.Info("Agent telemetry is stopped")
	return nil
}
