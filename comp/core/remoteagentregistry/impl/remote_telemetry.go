// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Remote agents report counters and histograms as running totals since the remote process started. Consumers of the
// Core Agent's telemetry (most notably Agent Telemetry, which turns counters into per-period deltas) assume instead
// that every counter starts from zero when the Core Agent starts. Passing the totals through unchanged therefore makes
// them over-report whenever the Core Agent restarts while the remote agent keeps running.
//
// The accumulator fixes this by accumulating the increases the Core Agent itself observes, per remote process. What it
// exposes is "everything counted since the Core Agent first saw this process", which behaves like the Core Agent's
// own counters. The only decision to make is what the first observation of a series is worth:
//
//   - In the first successful scrape of a process, everything already counted is history from before this Core Agent
//     knew the process, so it becomes the baseline and contributes nothing. The registry performs this scrape right
//     after the process registers, before it can have counted much.
//   - A series that first appears in a later scrape came into existence while the Core Agent was watching, so all of
//     its value is new.
//
// Counter resets are detected from a decrease (for example when the remote agent drops and later recreates an idle
// counter) and counted from zero. Totals of replaced processes are retained per series, so the exposed value of a
// series never goes down when a remote agent restarts.

const (
	// remoteTelemetrySeriesIdleTTL is how long a series can be absent from successful scrapes of its process (or its
	// process can be absent from the registry) before its state is folded into the retained totals.
	remoteTelemetrySeriesIdleTTL = 10 * time.Minute
	// remoteTelemetryRetiredTTL is how long retained totals are kept once nothing updates them.
	remoteTelemetryRetiredTTL = time.Hour
)

var (
	// The registry scrapes a process once right after registration to establish its baseline. The remote agent can
	// only answer once it has learned its session ID, which happens right after registration returns, so the scrape
	// is delayed and retried with a capped backoff. If every attempt fails, the first regular scrape is the baseline.
	telemetryBaselineInitialDelay = 200 * time.Millisecond
	telemetryBaselineMaxDelay     = 2 * time.Second
	telemetryBaselineAttempts     = 8
)

// exposedSeriesKey identifies a series as seen by consumers of the Core Agent's telemetry: it is independent of the
// remote process, so the series of a restarted remote agent continue the series of its predecessor.
type exposedSeriesKey struct {
	kind   dto.MetricType
	name   string
	labels string
}

type processSeriesKey struct {
	identity string
	series   exposedSeriesKey
}

// seriesValue holds the value of a counter (counter) or of a histogram (count, sum and cumulative buckets).
type seriesValue struct {
	counter float64
	count   uint64
	sum     float64
	buckets map[float64]uint64
}

func (v seriesValue) clone() seriesValue {
	out := v
	if v.buckets != nil {
		out.buckets = make(map[float64]uint64, len(v.buckets))
		for bound, count := range v.buckets {
			out.buckets[bound] = count
		}
	}
	return out
}

func sameBucketLayout(a, b map[float64]uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for bound := range a {
		if _, ok := b[bound]; !ok {
			return false
		}
	}
	return true
}

// addValue adds delta to total.
func addValue(kind dto.MetricType, total *seriesValue, delta seriesValue) {
	if kind == dto.MetricType_COUNTER {
		total.counter += delta.counter
		return
	}
	total.count += delta.count
	total.sum += delta.sum
	if total.buckets == nil {
		total.buckets = make(map[float64]uint64, len(delta.buckets))
	}
	for bound, count := range delta.buckets {
		total.buckets[bound] += count
	}
}

// advanceValue accumulates into total what a series gained between the observations last and cur.
func advanceValue(kind dto.MetricType, total *seriesValue, last, cur seriesValue) {
	if kind == dto.MetricType_COUNTER {
		if cur.counter >= last.counter {
			total.counter += cur.counter - last.counter
		} else {
			// A decrease means the counter was reset, so everything it holds now is new.
			total.counter += cur.counter
		}
		return
	}

	if !sameBucketLayout(last.buckets, cur.buckets) {
		// Different buckets can't be accumulated into one histogram: start a new one with the new layout.
		*total = cur.clone()
		return
	}
	reset := cur.count < last.count
	for bound, count := range cur.buckets {
		if count < last.buckets[bound] {
			reset = true
		}
	}
	if reset {
		addValue(kind, total, cur)
		return
	}
	delta := seriesValue{count: cur.count - last.count, sum: cur.sum - last.sum, buckets: make(map[float64]uint64, len(cur.buckets))}
	for bound, count := range cur.buckets {
		delta.buckets[bound] = count - last.buckets[bound]
	}
	addValue(kind, total, delta)
}

// seriesMeta is what is needed to emit a series.
type seriesMeta struct {
	help        string
	labelNames  []string
	labelValues []string
}

// observedSeries is a counter or histogram series as reported by one scrape of a remote process.
type observedSeries struct {
	key   exposedSeriesKey
	meta  seriesMeta
	value seriesValue
}

type liveSeries struct {
	meta     seriesMeta
	last     seriesValue
	total    seriesValue
	lastSeen time.Time
}

type retiredSeries struct {
	meta      seriesMeta
	total     seriesValue
	retiredAt time.Time
}

type processBaseline struct {
	baselined bool
	lastSeen  time.Time
}

// remoteTelemetryAccumulator accumulates the counters and histograms of remote agents. See the comment at the top of
// the file.
type remoteTelemetryAccumulator struct {
	// scrapeMu serializes scrapes, so that observations of a process are applied in the order they were fetched. An
	// observation applied out of order would look like a counter reset.
	scrapeMu sync.Mutex

	mu        sync.Mutex
	processes map[string]*processBaseline
	live      map[processSeriesKey]*liveSeries
	retired   map[exposedSeriesKey]*retiredSeries
}

func newRemoteTelemetryAccumulator() *remoteTelemetryAccumulator {
	return &remoteTelemetryAccumulator{
		processes: make(map[string]*processBaseline),
		live:      make(map[processSeriesKey]*liveSeries),
		retired:   make(map[exposedSeriesKey]*retiredSeries),
	}
}

func (r *remoteTelemetryAccumulator) isBaselined(identity string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.processes[identity]
	return p != nil && p.baselined
}

// observe applies one successful scrape of the process with the given identity.
func (r *remoteTelemetryAccumulator) observe(identity string, observed []observedSeries, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	process := r.processes[identity]
	if process == nil {
		process = &processBaseline{}
		r.processes[identity] = process
	}
	baselining := !process.baselined

	for _, s := range observed {
		key := processSeriesKey{identity: identity, series: s.key}
		series := r.live[key]
		switch {
		case series == nil:
			series = &liveSeries{last: s.value.clone()}
			if !baselining {
				series.total = s.value.clone()
			}
			r.live[key] = series
		default:
			advanceValue(s.key.kind, &series.total, series.last, s.value)
			series.last = s.value.clone()
		}
		series.meta = s.meta
		series.lastSeen = now
	}

	process.baselined = true
	process.lastSeen = now
}

// expire folds the state of series that went away into the retained totals and drops state that is no longer needed.
// registered holds the identities of the processes registered now, and scraped those that were just scraped
// successfully.
func (r *remoteTelemetryAccumulator) expire(registered, scraped map[string]struct{}, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hasLive := make(map[exposedSeriesKey]struct{})
	liveProcesses := make(map[string]struct{})
	for key, series := range r.live {
		_, isRegistered := registered[key.identity]
		_, wasScraped := scraped[key.identity]
		// A failed scrape of a registered process says nothing about its series.
		if now.Sub(series.lastSeen) >= remoteTelemetrySeriesIdleTTL && (!isRegistered || wasScraped) {
			r.retire(key.series, series, now)
			delete(r.live, key)
			continue
		}
		hasLive[key.series] = struct{}{}
		liveProcesses[key.identity] = struct{}{}
	}

	for key, series := range r.retired {
		// Dropping retained totals that a live series builds on would make the exposed value go down.
		if _, ok := hasLive[key]; !ok && now.Sub(series.retiredAt) >= remoteTelemetryRetiredTTL {
			delete(r.retired, key)
		}
	}

	for identity, process := range r.processes {
		_, isRegistered := registered[identity]
		_, hasSeries := liveProcesses[identity]
		if !isRegistered && !hasSeries && now.Sub(process.lastSeen) >= remoteTelemetrySeriesIdleTTL {
			delete(r.processes, identity)
		}
	}
}

// retire adds the accumulated total of a live series to the retained total of its exposed series. r.mu must be held.
func (r *remoteTelemetryAccumulator) retire(key exposedSeriesKey, series *liveSeries, now time.Time) {
	retired := r.retired[key]
	if retired == nil {
		r.retired[key] = &retiredSeries{meta: series.meta, total: series.total.clone(), retiredAt: now}
		return
	}
	if key.kind == dto.MetricType_HISTOGRAM && !sameBucketLayout(retired.total.buckets, series.total.buckets) {
		retired.total = series.total.clone()
	} else {
		addValue(key.kind, &retired.total, series.total)
	}
	retired.meta = series.meta
	retired.retiredAt = now
}

// emit passes the exposed value of every series to emit. The exposed value of a series is the retained total of its
// retired processes plus the totals of the processes currently reporting it.
func (r *remoteTelemetryAccumulator) emit(canonicalHelp func(string) (string, bool), emit func(prometheus.Metric)) {
	type exposed struct {
		meta        seriesMeta
		total       seriesValue
		seenAt      time.Time
		fromRetired bool
	}

	r.mu.Lock()
	totals := make(map[exposedSeriesKey]*exposed, len(r.retired))
	for key, series := range r.retired {
		totals[key] = &exposed{meta: series.meta, total: series.total.clone(), seenAt: series.retiredAt, fromRetired: true}
	}

	// Walk the live series in a fixed order, so what is exposed never depends on the iteration order of a map.
	keys := make([]processSeriesKey, 0, len(r.live))
	for key := range r.live {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.identity != b.identity {
			return a.identity < b.identity
		}
		if a.series.kind != b.series.kind {
			return a.series.kind < b.series.kind
		}
		if a.series.name != b.series.name {
			return a.series.name < b.series.name
		}
		return a.series.labels < b.series.labels
	})

	for _, key := range keys {
		series := r.live[key]
		e := totals[key.series]
		switch {
		case e == nil:
			totals[key.series] = &exposed{meta: series.meta, total: series.total.clone(), seenAt: series.lastSeen}
		case key.series.kind == dto.MetricType_HISTOGRAM && !sameBucketLayout(e.total.buckets, series.total.buckets):
			// Totals with different buckets can't be added. The newer report wins: a live series over a retained
			// total, and among live series the one seen last.
			if e.fromRetired || series.lastSeen.After(e.seenAt) {
				*e = exposed{meta: series.meta, total: series.total.clone(), seenAt: series.lastSeen}
			}
		default:
			addValue(key.series.kind, &e.total, series.total)
			e.meta = series.meta
			e.seenAt = series.lastSeen
			e.fromRetired = false
		}
	}
	r.mu.Unlock()

	for key, e := range totals {
		help := e.meta.help
		if canonicalHelp != nil {
			if canonical, found := canonicalHelp(key.name); found {
				help = canonical
			}
		}
		desc := prometheus.NewDesc(key.name, help, e.meta.labelNames, nil)
		var metric prometheus.Metric
		var err error
		if key.kind == dto.MetricType_COUNTER {
			metric, err = prometheus.NewConstMetric(desc, prometheus.CounterValue, e.total.counter, e.meta.labelValues...)
		} else {
			metric, err = prometheus.NewConstHistogram(desc, e.total.count, e.total.sum, e.total.buckets, e.meta.labelValues...)
		}
		if err != nil {
			log.Warnf("Failed to collect telemetry metric %v for remoteAgent %v: %v", key.name, e.meta.labelValues[0], err)
			continue
		}
		emit(metric)
	}
}

// parsedTelemetry is the content of one remote agent's telemetry payload.
type parsedTelemetry struct {
	// series are the counters and histograms, which are rebased before being exposed.
	series []observedSeries
	// gauges are exposed as they are.
	gauges []prometheus.Metric
}

// parsePromText parses the Prometheus text exposition a remote agent returned.
func parsePromText(promText string, remoteAgentName string, canonicalMetricHelp func(string) (string, bool)) (parsedTelemetry, bool) {
	var out parsedTelemetry

	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse prometheus text: %v", err)
		return out, false
	}

	seen := make(map[exposedSeriesKey]struct{})
	for _, mf := range metricFamilies {
		help := ""
		if mf.Help != nil {
			help = *mf.Help
		}
		// Prometheus requires one HELP string per metric family, so same-name local and remote
		// metrics with divergent HELP make the combined gather fail. The injected emitter label
		// separates samples but does not affect family metadata.
		if canonicalHelp, found := canonicalMetricHelp(mf.GetName()); found {
			help = canonicalHelp
		}

		for _, metric := range mf.Metric {
			if metric == nil {
				continue
			}

			labelNames, labelValues := canonicalMetricLabels(metric.Label, remoteAgentName)
			desc := prometheus.NewDesc(*mf.Name, help, labelNames, nil)

			switch *mf.Type {
			case dto.MetricType_COUNTER:
				value := *metric.Counter.Value

				// Only well-formed series enter the accumulator, so a malformed one can't linger in its state.
				if _, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, value, labelValues...); err != nil {
					log.Warnf("Failed to collect telemetry counter metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				out.addSeries(seen, mf, dto.MetricType_COUNTER, help, labelNames, labelValues, seriesValue{counter: value})
			case dto.MetricType_GAUGE:
				value := *metric.Gauge.Value

				m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry gauge metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				out.gauges = append(out.gauges, m)

			case dto.MetricType_SUMMARY:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unimplemented summary aggregation logic", mf.GetName(), remoteAgentName)
				continue

			case dto.MetricType_HISTOGRAM:
				count := metric.Histogram.GetSampleCount()
				sum := metric.Histogram.GetSampleSum()
				buckets := make(map[float64]uint64)
				for _, bucket := range metric.Histogram.GetBucket() {
					buckets[bucket.GetUpperBound()] = bucket.GetCumulativeCount()
				}

				if _, err := prometheus.NewConstHistogram(desc, count, sum, buckets, labelValues...); err != nil {
					log.Warnf("Failed to collect telemetry histogram metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				out.addSeries(seen, mf, dto.MetricType_HISTOGRAM, help, labelNames, labelValues, seriesValue{count: count, sum: sum, buckets: buckets})

			default:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unknown metric type %s", mf.GetName(), remoteAgentName, mf.GetType())
			}
		}
	}

	return out, true
}

func (p *parsedTelemetry) addSeries(seen map[exposedSeriesKey]struct{}, mf *dto.MetricFamily, kind dto.MetricType, help string, labelNames, labelValues []string, value seriesValue) {
	key := exposedSeriesKey{kind: kind, name: mf.GetName(), labels: encodeLabels(labelNames, labelValues)}
	if _, dup := seen[key]; dup {
		log.Warnf("Dropping duplicate telemetry series %v from remoteAgent %v", mf.GetName(), labelValues[0])
		return
	}
	seen[key] = struct{}{}
	p.series = append(p.series, observedSeries{
		key:   key,
		meta:  seriesMeta{help: help, labelNames: labelNames, labelValues: labelValues},
		value: value,
	})
}

// encodeLabels encodes a label set independently of the order of the labels.
func encodeLabels(names, values []string) string {
	idx := make([]int, len(names))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return names[idx[a]] < names[idx[b]] })
	var sb strings.Builder
	for _, i := range idx {
		sb.WriteString(names[i])
		sb.WriteByte(0xff)
		sb.WriteString(values[i])
		sb.WriteByte(0xfe)
	}
	return sb.String()
}

// telemetryScrape is the result of querying one remote agent's telemetry.
type telemetryScrape struct {
	agent    remoteagentregistry.RegisteredAgent
	identity string
	parsed   parsedTelemetry
	ok       bool
}

// collectTelemetry scrapes the registered remote agents accepted by agentFilter, accounts for what they report, and
// passes the current value of all their metrics to emit.
func (ra *remoteAgentRegistry) collectTelemetry(agentFilter func(remoteagentregistry.RegisteredAgent) bool, emit func(prometheus.Metric)) {
	r := ra.telemetryAccumulator
	r.scrapeMu.Lock()
	defer r.scrapeMu.Unlock()

	scraped := ra.scrapeAndObserve(agentFilter, emit)

	registered := make(map[string]struct{})
	for _, agent := range ra.GetRegisteredAgents() {
		registered[remoteAgentIdentity(agent)] = struct{}{}
	}
	r.expire(registered, scraped, time.Now())
	r.emit(ra.telemetry.CanonicalMetricHelp, emit)
}

// scrapeAndObserve queries the remote agents and applies what they report. It returns the identities of the processes
// that were scraped successfully. r.scrapeMu must be held.
func (ra *remoteAgentRegistry) scrapeAndObserve(agentFilter func(remoteagentregistry.RegisteredAgent) bool, emitGauge func(prometheus.Metric)) map[string]struct{} {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
		return remoteAgent.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetTelemetryResponse, err error) telemetryScrape {
		if err != nil {
			log.Warnf("Failed to collect telemetry metrics from remoteAgent %v: %v", details.SanitizedDisplayName, err)
			return telemetryScrape{}
		}
		promText, isPromText := resp.Payload.(*pb.GetTelemetryResponse_PromText)
		if !isPromText {
			// Nothing to report, but the process answered.
			return telemetryScrape{agent: details, identity: remoteAgentIdentity(details), ok: true}
		}
		parsed, parsedOK := parsePromText(promText.PromText, details.SanitizedDisplayName, ra.telemetry.CanonicalMetricHelp)
		if !parsedOK {
			return telemetryScrape{}
		}
		return telemetryScrape{agent: details, identity: remoteAgentIdentity(details), parsed: parsed, ok: true}
	}

	scrapes := callAgentsForServiceFiltered(ra.ctx, ra, TelemetryServiceName, agentFilter, client, processor)

	now := time.Now()
	scraped := make(map[string]struct{})
	for _, scrape := range scrapes {
		if !scrape.ok {
			continue
		}
		scraped[scrape.identity] = struct{}{}
		ra.telemetryAccumulator.observe(scrape.identity, scrape.parsed.series, now)
		for _, gauge := range scrape.parsed.gauges {
			emitGauge(gauge)
		}
	}
	return scraped
}

// newestSessionFilter accepts only the most recently registered session of each remote process. A process can
// briefly have two sessions after re-registering, and both report the same counters.
func (ra *remoteAgentRegistry) newestSessionFilter() func(remoteagentregistry.RegisteredAgent) bool {
	newest := make(map[string]string)
	orders := make(map[string]uint64)
	ra.agentMapMu.Lock()
	for _, agent := range ra.agentMap {
		identity := remoteAgentIdentity(agent.RegisteredAgent)
		if agent.registrationOrder >= orders[identity] {
			newest[identity] = agent.SessionID
			orders[identity] = agent.registrationOrder
		}
	}
	ra.agentMapMu.Unlock()
	return func(agent remoteagentregistry.RegisteredAgent) bool {
		return newest[remoteAgentIdentity(agent)] == agent.SessionID
	}
}

// primeTelemetryBaseline scrapes the process behind a new session once, so that the totals it already holds become
// its baseline. It retries for a short while, because the remote agent can only answer once it knows its session ID.
func (ra *remoteAgentRegistry) primeTelemetryBaseline(sessionID string) {
	delay := ra.telemetryBaselineDelay
	for attempt := 0; attempt < telemetryBaselineAttempts; attempt++ {
		select {
		case <-ra.ctx.Done():
			return
		case <-time.After(delay):
		}
		if ra.primeTelemetryBaselineOnce(sessionID) {
			return
		}
		delay = min(delay*2, telemetryBaselineMaxDelay)
	}
	log.Debugf("Could not scrape the telemetry of the remote agent session %s after registration; the first regular scrape will be its baseline.", sessionID)
}

// primeTelemetryBaselineOnce reports whether there is nothing left to do for the session.
//
// Failing is expected here: the remote agent can't answer until it knows its session ID. So unlike regular scrapes,
// this one doesn't count failures as errors, doesn't mark the agent unhealthy over a session ID that doesn't match
// yet, logs at debug level, and doesn't wait for the connection to become ready, which would hold up other scrapes.
func (ra *remoteAgentRegistry) primeTelemetryBaselineOnce(sessionID string) bool {
	r := ra.telemetryAccumulator
	r.scrapeMu.Lock()
	defer r.scrapeMu.Unlock()

	ra.agentMapMu.Lock()
	client, found := ra.agentMap[sessionID]
	var agent remoteagentregistry.RegisteredAgent
	if found {
		agent = client.RegisteredAgent
	}
	ra.agentMapMu.Unlock()
	identity := remoteAgentIdentity(agent)
	// The session is gone, or the process already has a baseline from an earlier session.
	if !found || r.isBaselined(identity) {
		return true
	}

	ctx, cancel := context.WithTimeout(ra.ctx, ra.conf.GetDuration("remote_agent.registry.query_timeout"))
	defer cancel()
	var header metadata.MD
	resp, err := client.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, grpc.Header(&header))
	if err == nil {
		err = client.validateSessionID(header)
	}
	if err != nil {
		log.Debugf("Could not scrape the telemetry of remote agent %v after registration yet: %v", agent.SanitizedDisplayName, err)
		return false
	}

	var series []observedSeries
	if promText, isPromText := resp.Payload.(*pb.GetTelemetryResponse_PromText); isPromText {
		parsed, ok := parsePromText(promText.PromText, agent.SanitizedDisplayName, ra.telemetry.CanonicalMetricHelp)
		if !ok {
			return false
		}
		series = parsed.series
	}
	r.observe(identity, series, time.Now())
	return true
}
