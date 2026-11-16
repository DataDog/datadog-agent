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
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
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

	enabled bool
	sender  sender
	runner  runner
	atelCfg *Config

	localEmitter string

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
	// errLogsCh is allocated only while errortracking is on, so deployments
	// that don't opt in pay no buffer. It is switched at runtime by
	// setErrortracking, which serializes with start and stop on
	// errortrackingMu and guards the fields below it.
	errLogsCh            atomic.Pointer[chan errortracking.ErrorLog]
	errLogsBufferSize    int
	errLogsDropped       *atomic.Uint64
	errLogsFlushInterval time.Duration
	errLogsStartupJitter time.Duration
	shutdownDrainTimeout time.Duration
	// errLogsFlushFailures counts consecutive failed errortracking flushes,
	// the health signal of the errortracking remote flag.
	errLogsFlushFailures atomic.Uint32

	errortrackingMu sync.Mutex
	// errortrackingSyncMu serializes the reaction to configuration changes,
	// which reads the key and applies it.
	errortrackingSyncMu sync.Mutex
	// errortrackingDrains tracks the flushes of queues released by a switch
	// off, which stop waits for.
	errortrackingDrains sync.WaitGroup
	errortrackingJob    bool
	started             bool
	stopped             bool

	// flag gates profiles carrying a `remote_flag`; nil means every gated profile is off.
	flag *remoteFlagHandler

	// errortrackingFlag turns errortracking on and off at runtime.
	errortrackingFlag *errortrackingFlagHandler

	// flushFailures counts consecutive flushSession failures, the health signal for remote flags.
	flushFailures *atomic.Uint32
}

const (
	emitterTagName                               = "emitter"
	defaultEmitter                               = "agent"
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
	// Subscriber registers this component's remote flag handlers.
	Subscriber pkgremoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
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

func createAtel(
	cfgComp config.Component,
	logComp log.Component,
	telComp telemetry.Component,
	sender sender,
	runner runner) *atel {
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

	// Errortracking starts on when the configuration enables it; later changes
	// of the key, such as the remote flag, switch it (see setErrortracking). The gate
	// composes with IsAgentTelemetryEnabled so gov/FIPS sites (parent
	// agent_telemetry is excluded for them) automatically opt out.
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

	// Final-drain context budget — bounded so a hung intake cannot
	// block agent shutdown. See atel.stop for usage.
	shutdownDrainTimeoutSeconds := getNonNegativeErrortrackingInt(
		cfgComp,
		logComp,
		"agent_telemetry.errortracking.shutdown_drain_timeout_seconds",
		defaultErrortrackingShutdownDrainTimeoutSecs,
	)

	a := &atel{
		enabled: true,
		cfgComp: cfgComp,
		logComp: logComp,
		telComp: telComp,
		sender:  sender,
		runner:  runner,
		atelCfg: atelCfg,

		lightTracer: installertelemetry.NewTelemetry(
			tracerHTTPClient,
			utils.SanitizeAPIKey(cfgComp.GetString("api_key")),
			cfgComp.GetString("site"),
			"datadog-agent",
		),

		prevPromMetricCounterValues:   make(map[string]float64),
		prevPromMetricHistogramValues: make(map[string]uint64),

		flushFailures: atomic.NewUint32(0),

		errLogsBufferSize:    bufferSize,
		errLogsDropped:       atomic.NewUint64(0),
		errLogsFlushInterval: time.Duration(flushIntervalSeconds) * time.Second,
		errLogsStartupJitter: time.Duration(startupJitterSeconds) * time.Second,
		shutdownDrainTimeout: time.Duration(shutdownDrainTimeoutSeconds) * time.Second,
	}
	if errortrackingEnabled {
		a.allocErrortrackingQueueLocked()
	}
	return a
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
	)
	localEmitter := strings.ReplaceAll(flavor.GetFlavor(), "_", "-")
	if localEmitter == "" {
		localEmitter = defaultEmitter
	}
	a.localEmitter = localEmitter

	a.flag = newRemoteFlagHandler(flagTroubleshooting, a.isHealthy)
	a.errortrackingFlag = newErrortrackingFlagHandler(deps.Log, a.isErrortrackingHealthy)

	// Register synchronously, not on start, so errors logged while the
	// component graph is still being built are not dropped.
	if a.errLogsCh.Load() != nil {
		a.publishErrortracking()
	}

	// If agent telemetry is enabled and configured properly add the start and stop hooks
	if a.enabled {
		// Errortracking follows its configuration key after startup: the remote
		// flag drives it through the Remote Config source. The sync covers a
		// change between createAtel reading the key and the callback existing.
		deps.Config.OnUpdate(a.onConfigUpdate)
		a.syncErrortracking()
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
		Comp:       a,
		Endpoint:   api.NewAgentEndpointProvider(a.writePayload, "/metadata/agent-telemetry", "GET"),
		Subscriber: remoteFlagSubscriber{a.flag, a.errortrackingFlag},
	}
}

func (a *atel) effectiveEmitter(labels []*dto.LabelPair) string {
	for _, label := range labels {
		if label.GetName() == emitterTagName && label.GetValue() != "" {
			return label.GetValue()
		}
	}
	if a.localEmitter != "" {
		return a.localEmitter
	}
	return defaultEmitter
}

func (a *atel) aggregateMetricTags(mCfg *MetricConfig, mt dto.MetricType, ms []*dto.Metric) []*dto.Metric {
	// Nothing to aggregate?
	if len(ms) == 0 {
		return nil
	}

	amMap := make(map[string]*dto.Metric)

	// Initialize total metrics
	var totalm map[string]*dto.Metric
	var totalCount map[string]int
	if mCfg.AggregateTotal {
		totalm = make(map[string]*dto.Metric)
		totalCount = make(map[string]int)
	}

	// Enumerate the metric's timeseries and aggregate them
	for _, m := range ms {
		emitter := a.effectiveEmitter(m.GetLabel())
		emitterName := emitterTagName
		emitterTag := &dto.LabelPair{Name: &emitterName, Value: &emitter}
		specTags := []*dto.LabelPair{emitterTag}

		// If tags are defined, drop those not specified in the configuration.
		origTags := m.GetLabel()
		if len(origTags) > 0 && mCfg.preserveTagsExists {
			// sort tags (to have a consistent key for the same tag set)
			tags := cloneLabelsSorted(origTags)
			for _, t := range tags {
				if t.GetName() == emitterTagName {
					continue
				}
				if _, ok := mCfg.preserveTagsMap[t.GetName()]; ok {
					specTags = append(specTags, t)
				}
			}
		}
		tagsKey := encodeSortedLabels([]*dto.LabelPair{emitterTag}) + encodeSortedLabels(specTags[1:])

		if mCfg.AggregateTotal {
			if totalm[emitter] == nil {
				totalm[emitter] = &dto.Metric{}
			}
			aggregateMetric(mt, totalm[emitter], m)
			totalCount[emitter]++
		}

		// finally aggregate the metric on the created key
		if aggm, ok := amMap[tagsKey]; ok {
			aggregateMetric(mt, aggm, m)
		} else {
			// ... or create a new one with the specified value and tags
			aggm := &dto.Metric{}
			aggregateMetric(mt, aggm, m)
			aggm.Label = specTags
			amMap[tagsKey] = aggm
		}
	}

	// Add total metrics if needed
	for emitter, total := range totalm {
		emitterName := emitterTagName
		totalName := "total"
		totalValue := strconv.Itoa(totalCount[emitter])
		total.Label = []*dto.LabelPair{
			{Name: &emitterName, Value: &emitter},
			{Name: &totalName, Value: &totalValue},
		}
		amMap[encodeSortedLabels(total.Label)] = total
	}

	// Anything to report?
	if len(amMap) == 0 {
		return nil
	}

	// Convert the map to a slice
	return slices.Collect(maps.Values(amMap))
}

// Using Prometheus  terminology. Metrics name or in "Prom" MetricFamily is technically a Datadog metrics.
// dto.Metric are a metric values for each timeseries (tag/value combination).
func buildKeysForMetricsPreviousValues(mt dto.MetricType, metricName string, metrics []*dto.Metric) []string {
	keyNames := make([]string, 0, len(metrics))
	for _, m := range metrics {
		var keyName string
		tags := m.GetLabel()
		if len(tags) == 0 {
			// For "tagless" MetricFamily, len(metrics) will be 1, with single iteration and m.GetLabel()
			// will be nil. Accordingly, to form a key for that metric its name alone is sufficient.
			keyName = metricName
		} else {
			//If the metric has tags, len(metrics) will be equal to the number of metric's timeseries.
			// Each timeseries or "m" on each iteration in this code, will contain a set of unique
			// tagset (as m.GetLabel()). Accordingly, each timeseries should be represented by a unique
			// and stable (reproducible) key formed by tagset key names and values.
			sortedTags := cloneLabelsSorted(tags)
			keyName = fmt.Sprintf("%s%s:", metricName, encodeSortedLabels(sortedTags))
		}

		if mt == dto.MetricType_HISTOGRAM {
			// On each iteration for metrics without tags (only 1 iteration) or with tags (iteration per
			// timeseries). If the metric is a HISTOGRAM, each timeseries bucket individually plus
			// implicit "+Inf" bucket. For example, for 3 timeseries with 4-bucket histogram, we will
			// track 15 values using 15 keys (3x(4+1)).
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

		// A lower cumulative value marks a producer reset, so its current value is the delta.
		if prevValue, ok := prevPromMetricValues[key]; ok && curValue >= prevValue {
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

	// filter out if tag does not contain in existing preserveTags
	if len(mCfg.preserveTagsMap) > 0 && !areTagsMatching(m.GetLabel(), mCfg.preserveTagsMap) {
		return false
	}

	return true
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

	// Filter the metric according to the profile configuration
	// Currently we only support filtering out zero values if specified in the profile
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
	// Gather all prom metrics, then filter them per-profile below.
	pms, err := a.telComp.Gather(telemetry.NoFilter)
	if err != nil {
		a.logComp.Errorf("failed to get filtered telemetry metrics: %v", err)
		return nil, err
	}

	// All metrics stored in the "pms" slice above must follow the format:
	//    <subsystem>__<metric_name>
	// The "subsystem" and "name" should be concatenated with a double underscore ("__") separator,
	// e.g., "checks__execution_time". Therefore, the "Options.NoDoubleUnderscoreSep: true" option
	// must not be used when creating metrics.

	session := a.sender.startSession(a.cancelCtx)
	for _, p := range profiles {
		if p.RemoteFlag != "" && !a.flag.enabledFor(p.RemoteFlag) {
			continue
		}
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
	a.recordFlushResult(err)
	if err != nil {
		a.logComp.Errorf("failed to flush agent telemetry session: %s", err)
		return
	}
}

// recordFlushResult feeds the remote flag health signal: consecutive failures
// accumulate, any success clears them.
func (a *atel) recordFlushResult(err error) {
	if a.flushFailures == nil {
		return
	}
	if err != nil {
		a.flushFailures.Inc()
		return
	}
	a.flushFailures.Store(0)
}

// isHealthy is the health probe the Remote Flags client polls after a flag is
// enabled. Reporting false long enough makes the client call SafeRecover,
// which turns the gated profiles back off.
func (a *atel) isHealthy() bool {
	if !a.enabled {
		return false
	}
	if a.flushFailures == nil {
		return true
	}
	return a.flushFailures.Load() < maxConsecutiveFlushFailures
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
	ch := a.errLogsCh.Load()
	if ch == nil {
		// Errortracking is off: no channel allocated. Calling this should
		// be a no-op rather than a nil-channel send (which would block
		// forever).
		return
	}
	select {
	case *ch <- log:
	default:
		a.errLogsDropped.Add(1)
	}
}

// flushErrortracking drains logs currently buffered in errLogsCh and
// dispatches them as ONE HTTP call via sender.sendLogsBatch.
// The drain is bounded to len(errLogsCh) at flush start: items that
// arrive after the snapshot are left for the next tick, preventing a
// hot error stream from holding the flush job indefinitely.
// A no-op when errortracking is off (no channel) or the channel is empty.
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
	ch := a.errLogsCh.Load()
	if ch == nil {
		return
	}
	sent, err := a.flushErrortrackingQueue(ctx, *ch)
	// Only the live queue feeds the health signal: a switch off or on while
	// this flush ran makes its outcome irrelevant.
	if !sent || a.errLogsCh.Load() != ch {
		return
	}
	if err != nil {
		a.errLogsFlushFailures.Inc()
		return
	}
	a.errLogsFlushFailures.Store(0)
}

// flushErrortrackingQueue drains up to the current depth of ch and sends it.
// It reports whether a batch was sent and the send error.
func (a *atel) flushErrortrackingQueue(ctx context.Context, ch chan errortracking.ErrorLog) (bool, error) {
	// Snapshot the current queue depth so a hot producer cannot extend
	// this batch indefinitely: items arriving after the snapshot wait for
	// the next tick rather than growing errLogs beyond cap(ch).
	n := len(ch)
	if n == 0 {
		return false, nil
	}
	errLogs := make([]errortracking.ErrorLog, 0, n)
	for range n {
		select {
		case l := <-ch:
			errLogs = append(errLogs, l)
		default:
			// Channel drained before snapshot count — harmless, stop early.
			break
		}
	}
	if len(errLogs) == 0 {
		return false, nil
	}
	logs := make([]Log, len(errLogs))
	for i, l := range errLogs {
		logs[i] = enrichErrorLog(l)
	}
	err := a.sender.sendLogsBatch(ctx, logs)
	if err != nil {
		a.logComp.Debugf("errortracking flush failed (%d logs): %v", len(errLogs), err)
	}
	return true, err
}

// isErrortrackingHealthy is the health probe of the errortracking remote
// flag: consecutive failed flushes make it unhealthy. It is independent from
// isHealthy so one feature's failures never switch off the other.
func (a *atel) isErrortrackingHealthy() bool {
	return a.enabled && a.errLogsFlushFailures.Load() < maxConsecutiveFlushFailures
}

// allocErrortrackingQueueLocked allocates the bounded queue; the caller holds
// errortrackingMu or has exclusive access to a.
func (a *atel) allocErrortrackingQueueLocked() {
	ch := make(chan errortracking.ErrorLog, a.errLogsBufferSize)
	a.errLogsCh.Store(&ch)
}

// publishErrortracking routes the logger's error records to this component.
// The bouncer is created here so each activation starts with an empty window.
func (a *atel) publishErrortracking() {
	bouncerWindow := time.Duration(a.cfgComp.GetInt("agent_telemetry.errortracking.bouncer_window_seconds")) * time.Second
	pkglogsetup.RegisterErrortrackingBouncer(errortracking.NewBouncer(bouncerWindow, 0))
	pkglogsetup.RegisterErrortrackingSubmitter(a.SubmitErrorLog)
}

// unpublishErrortracking stops routing the logger's error records.
func unpublishErrortracking() {
	pkglogsetup.RegisterErrortrackingSubmitter(nil)
	pkglogsetup.RegisterErrortrackingBouncer(nil)
}

// setErrortracking switches errortracking on or off at runtime. It is a no-op
// when the component is disabled or stopped, or when the state is unchanged.
//
// Switching on allocates the queue, resets the health signal, schedules the
// flush job (once, it stays scheduled and idles while off) and routes the
// logger's records here. Switching off stops routing first, then flushes what
// is already buffered the way stop does, then releases the queue. Records a
// producer submitted concurrently with the switch may be dropped. Errors
// logged before the remote flag first arrives are not captured.
//
// The release of the queue runs in the background so that the Remote Flags
// client, which calls this under its own lock, is never held up by a slow
// intake; stop waits for it.
func (a *atel) setErrortracking(on bool) {
	a.errortrackingMu.Lock()
	if !a.enabled || a.stopped || (a.errLogsCh.Load() != nil) == on {
		a.errortrackingMu.Unlock()
		return
	}

	if on {
		defer a.errortrackingMu.Unlock()
		a.errLogsFlushFailures.Store(0)
		a.allocErrortrackingQueueLocked()
		a.addErrortrackingJobLocked()
		a.publishErrortracking()
		return
	}

	unpublishErrortracking()
	ch := a.errLogsCh.Swap(nil)
	a.errortrackingDrains.Add(1)
	a.errortrackingMu.Unlock()

	go func() {
		defer a.errortrackingDrains.Done()
		a.drainErrortracking(*ch)
	}()
}

// drainErrortracking sends what ch holds, bounded by the shutdown drain
// timeout and independent from the lifecycle context.
func (a *atel) drainErrortracking(ch chan errortracking.ErrorLog) {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownDrainTimeout)
	defer cancel()
	_, _ = a.flushErrortrackingQueue(ctx, ch)
}

// addErrortrackingJobLocked schedules the errortracking flush on the runner
// once the runner is started. The caller holds errortrackingMu.
func (a *atel) addErrortrackingJobLocked() {
	if !a.started || a.errortrackingJob {
		return
	}
	a.errortrackingJob = true

	flushPeriodSec := uint(a.errLogsFlushInterval / time.Second)
	if flushPeriodSec == 0 {
		// Guard against a zero flush interval from misconfiguration; Period:0
		// would schedule a degenerate job. Default is 60 s; floor at 5 s.
		a.logComp.Warnf("agent_telemetry.errortracking.flush_interval_seconds resolved to 0; clamping to 5 s")
		flushPeriodSec = 5
	}
	var startAfterSec uint
	if a.errLogsStartupJitter > 0 {
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

	// The errortracking flush is a runner job too, scheduled when the
	// feature is on at start or the first time it is switched on. profiles==nil
	// is the discriminator the job's Run dispatches on.
	a.errortrackingMu.Lock()
	defer a.errortrackingMu.Unlock()
	a.started = true
	if a.errLogsCh.Load() != nil {
		a.addErrortrackingJobLocked()
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
	a.errortrackingMu.Lock()
	a.stopped = true
	unpublishErrortracking()
	a.errortrackingMu.Unlock()

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
	a.errortrackingDrains.Wait()
	if ch := a.errLogsCh.Load(); ch != nil {
		a.drainErrortracking(*ch)
	}

	a.logComp.Info("Agent telemetry is stopped")
	return nil
}
