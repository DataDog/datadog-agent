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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/maps"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	dto "github.com/prometheus/client_model/go"
)

type atel struct {
	cfgComp config.Component
	logComp log.Component
	telComp telemetry.Component

	enabled bool
	sender  sender
	runner  runner
	atelCfg *Config

	lightTracer *installertelemetry.Telemetry

	cancelCtx context.Context
	cancel    context.CancelFunc

	startupSpan *installertelemetry.Span

	prevPromMetricCounterValues   map[string]float64
	prevPromMetricHistogramValues map[string]uint64
	prevPromMetricValuesMU        sync.Mutex
}

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
type job struct {
	a        *atel
	profiles []*Profile
	schedule Schedule
}

func (j job) Run() {
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

	return &atel{
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
	}
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

func (a *atel) aggregateMetricTags(mCfg *MetricConfig, mt dto.MetricType, ms []*dto.Metric) []*dto.Metric {
	// Nothing to aggregate?
	if len(ms) == 0 {
		return nil
	}

	// Special case when no aggregate tags are defined - aggregate all metrics
	// aggregateMetric will sum all metrics into a single one without copying tags
	if !mCfg.aggregateTagsExists {
		ma := &dto.Metric{}
		for _, m := range ms {
			aggregateMetric(mt, ma, m)
		}

		return []*dto.Metric{ma}
	}

	amMap := make(map[string]*dto.Metric)

	// Initialize total metric
	var totalm *dto.Metric
	if mCfg.AggregateTotal {
		totalm = &dto.Metric{}
	}

	// Enumerate the metric's timeseries and aggregate them
	for _, m := range ms {
		tagsKey := ""

		// if tags are defined, we need to create a key from them by dropping not specified
		// in configuration tags. The key is constructed by concatenating specified tag names
		// and values if a timeseries has tags is not specified
		origTags := m.GetLabel()
		if len(origTags) > 0 {
			// sort tags (to have a consistent key for the same tag set)
			tags := cloneLabelsSorted(origTags)

			// create a key from the tags (and drop not specified in the configuration tags)
			var specTags = make([]*dto.LabelPair, 0, len(origTags))
			var sb strings.Builder
			for _, t := range tags {
				if _, ok := mCfg.aggregateTagsMap[t.GetName()]; ok {
					specTags = append(specTags, t)
					sb.WriteString(makeLabelPairKey(t))
				}
			}
			tagsKey = sb.String()

			if mCfg.AggregateTotal {
				aggregateMetric(mt, totalm, m)
			}

			// finally aggregate the metric on the created key
			if aggm, ok := amMap[tagsKey]; ok {
				aggregateMetric(mt, aggm, m)
			} else {
				// ... or create a new one with specifi value and specified tags
				aggm := &dto.Metric{}
				aggregateMetric(mt, aggm, m)
				aggm.Label = specTags
				amMap[tagsKey] = aggm
			}
		} else {
			// if no tags are specified, we aggregate all metrics into a single one
			if mCfg.AggregateTotal {
				aggregateMetric(mt, totalm, m)
			}
		}
	}

	// Add total metric if needed
	if mCfg.AggregateTotal {
		totalName := "total"
		totalValue := strconv.Itoa(len(ms))
		totalm.Label = []*dto.LabelPair{
			{Name: &totalName, Value: &totalValue},
		}
		amMap[totalName] = totalm
	}

	// Anything to report?
	if len(amMap) == 0 {
		return nil
	}

	// Convert the map to a slice
	return maps.Values(amMap)
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
			keyName = fmt.Sprintf("%s%s:", metricName, convertLabelsToKey(tags))
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

	// filter out if tag does not contain in existing aggregateTags
	if mCfg.aggregateTagsExists && !areTagsMatching(m.GetLabel(), mCfg.aggregateTagsMap) {
		return false
	}

	return true
}

func (a *atel) transformMetricFamily(p *Profile, mfam *dto.MetricFamily) *agentmetric {
	var mCfg *MetricConfig
	var ok bool

	// Check if the metric is included in the profile
	if mCfg, ok = p.metricsMap[mfam.GetName()]; !ok {
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

	return nil
}

// stop is called by FX when the application stops.
func (a *atel) stop() error {
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

	<-a.cancelCtx.Done()
	a.logComp.Info("Agent telemetry is stopped")
	return nil
}
