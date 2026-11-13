// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"slices"
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

// Remote agents report counters and histograms as totals since their own process started, but consumers such as Agent
// Telemetry assume counters start at zero when the Core Agent starts. Forwarding the totals as they are makes every
// counter spike after a Core Agent restart that the remote agent survives.
//
// The accumulator exposes what the Core Agent has seen counted since it first saw each series instead: the first
// scrape of a process only sets a baseline, a series that shows up later counts in full, a decrease is a reset, and a
// restarted process takes its series over from the old one.

const (
	// remoteTelemetrySeriesIdleTTL is how long a series can go unreported, by a process that keeps answering or that is
	// gone, before its state is dropped.
	remoteTelemetrySeriesIdleTTL = 10 * time.Minute
	// remoteTelemetryExpireInterval is how often the state is checked for expiry. The TTL is far longer than the
	// interval between scrapes, so there is no need to check on every one.
	remoteTelemetryExpireInterval = time.Minute
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

// zeroLike returns an empty value with the same buckets.
func (v seriesValue) zeroLike() seriesValue {
	var out seriesValue
	if v.buckets != nil {
		out.buckets = make(map[float64]uint64, len(v.buckets))
		for bound := range v.buckets {
			out.buckets[bound] = 0
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
		// Different buckets can't be accumulated into one histogram: start a new one with the new layout. The exposed
		// value goes down once.
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

// observedSeries is a counter or histogram series as reported by one scrape of a remote process.
type observedSeries struct {
	key         exposedSeriesKey
	help        string
	labelNames  []string
	labelValues []string
	value       seriesValue
}

// remoteSeries is the state of one series.
type remoteSeries struct {
	// owner is the identity of the process the series follows.
	owner string
	// last is the value the owner reported last, and total what was accounted since the series was first seen.
	last  seriesValue
	total seriesValue
	// lastSeen is when the owner last reported the series.
	lastSeen time.Time

	// What is needed to build the metric. The descriptor is built once, since it does not change.
	help        string
	desc        *prometheus.Desc
	labelNames  []string
	labelValues []string
}

type processBaseline struct {
	baselined bool
	lastSeen  time.Time
}

// remoteTelemetryAccumulator accumulates the counters and histograms of remote agents. See the comment at the top of
// the file.
type remoteTelemetryAccumulator struct {
	// scrapeMu serializes scrapes, so that observations of a process are applied in the order they were fetched. An
	// observation applied out of order would look like a counter reset. It also guards lastExpire.
	scrapeMu   sync.Mutex
	lastExpire time.Time

	mu        sync.Mutex
	processes map[string]*processBaseline
	series    map[exposedSeriesKey]*remoteSeries
}

func newRemoteTelemetryAccumulator() *remoteTelemetryAccumulator {
	return &remoteTelemetryAccumulator{
		processes: make(map[string]*processBaseline),
		series:    make(map[exposedSeriesKey]*remoteSeries),
	}
}

func (r *remoteTelemetryAccumulator) isBaselined(identity string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.processes[identity]
	return p != nil && p.baselined
}

// observe applies one successful scrape of the process with the given identity. It takes ownership of observed.
func (r *remoteTelemetryAccumulator) observe(identity string, observed []observedSeries, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	process := r.processes[identity]
	if process == nil {
		process = &processBaseline{}
		r.processes[identity] = process
	}
	baselining := !process.baselined

	for i := range observed {
		s := &observed[i]
		series := r.series[s.key]
		switch {
		case series == nil:
			series = &remoteSeries{owner: identity, last: s.value, labelNames: s.labelNames, labelValues: s.labelValues}
			if baselining {
				series.total = s.value.zeroLike()
			} else {
				// The series came into existence while the process was being watched: all of it is new.
				series.total = s.value.clone()
			}
			r.series[s.key] = series
		case series.owner != identity:
			// Another process reports the series now, typically because the remote agent restarted. What it counted
			// since its own start says nothing about what was accounted so far, so the series follows it from here:
			// the first scrape of a new process is only a baseline, and a series it starts reporting later counts in
			// full.
			series.owner = identity
			if s.key.kind == dto.MetricType_HISTOGRAM && !sameBucketLayout(series.last.buckets, s.value.buckets) {
				// The new process has other buckets, which can't be added to what was accounted so far.
				series.total = s.value.zeroLike()
			}
			if !baselining {
				addValue(s.key.kind, &series.total, s.value)
			}
			series.last = s.value
		default:
			advanceValue(s.key.kind, &series.total, series.last, s.value)
			series.last = s.value
		}
		if series.desc == nil || series.help != s.help {
			series.help = s.help
			series.desc = prometheus.NewDesc(s.key.name, s.help, series.labelNames, nil)
		}
		series.lastSeen = now
	}

	process.baselined = true
	process.lastSeen = now
}

// expire drops the state of series that went away and of processes nothing refers to anymore. registered holds the
// identities of the processes registered now, and scraped those that were just scraped successfully.
func (r *remoteTelemetryAccumulator) expire(registered, scraped map[string]struct{}, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	owners := make(map[string]struct{})
	for key, series := range r.series {
		_, isRegistered := registered[series.owner]
		_, wasScraped := scraped[series.owner]
		// A failed scrape of a registered process says nothing about its series.
		if now.Sub(series.lastSeen) >= remoteTelemetrySeriesIdleTTL && (!isRegistered || wasScraped) {
			delete(r.series, key)
			continue
		}
		owners[series.owner] = struct{}{}
	}

	for identity, process := range r.processes {
		_, isRegistered := registered[identity]
		_, owns := owners[identity]
		if !isRegistered && !owns && now.Sub(process.lastSeen) >= remoteTelemetrySeriesIdleTTL {
			delete(r.processes, identity)
		}
	}
}

// emit passes the exposed value of every series to emit.
func (r *remoteTelemetryAccumulator) emit(emit func(prometheus.Metric)) {
	r.mu.Lock()
	metrics := make([]prometheus.Metric, 0, len(r.series))
	for key, series := range r.series {
		var metric prometheus.Metric
		var err error
		if key.kind == dto.MetricType_COUNTER {
			metric, err = prometheus.NewConstMetric(series.desc, prometheus.CounterValue, series.total.counter, series.labelValues...)
		} else {
			// The metric keeps the buckets and reads them later, while the total keeps changing: give it a copy.
			total := series.total.clone()
			metric, err = prometheus.NewConstHistogram(series.desc, total.count, total.sum, total.buckets, series.labelValues...)
		}
		if err != nil {
			log.Warnf("Failed to collect telemetry metric %v for remoteAgent %v: %v", key.name, series.labelValues[0], err)
			continue
		}
		metrics = append(metrics, metric)
	}
	r.mu.Unlock()

	// Hand the metrics over without holding the lock: the receiver may block.
	for _, metric := range metrics {
		emit(metric)
	}
}

// parsedTelemetry is the content of one remote agent's telemetry payload.
type parsedTelemetry struct {
	// series are the counters and histograms, which are accumulated before being exposed.
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

			switch *mf.Type {
			case dto.MetricType_COUNTER:
				out.addSeries(seen, mf, dto.MetricType_COUNTER, help, labelNames, labelValues, seriesValue{counter: metric.GetCounter().GetValue()})

			case dto.MetricType_GAUGE:
				desc := prometheus.NewDesc(*mf.Name, help, labelNames, nil)
				m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, metric.GetGauge().GetValue(), labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry gauge metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				out.gauges = append(out.gauges, m)

			case dto.MetricType_SUMMARY:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unimplemented summary aggregation logic", mf.GetName(), remoteAgentName)
				continue

			case dto.MetricType_HISTOGRAM:
				buckets := make(map[float64]uint64, len(metric.GetHistogram().GetBucket()))
				for _, bucket := range metric.GetHistogram().GetBucket() {
					buckets[bucket.GetUpperBound()] = bucket.GetCumulativeCount()
				}
				out.addSeries(seen, mf, dto.MetricType_HISTOGRAM, help, labelNames, labelValues,
					seriesValue{count: metric.GetHistogram().GetSampleCount(), sum: metric.GetHistogram().GetSampleSum(), buckets: buckets})

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
	p.series = append(p.series, observedSeries{key: key, help: help, labelNames: labelNames, labelValues: labelValues, value: value})
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

	if now := time.Now(); now.Sub(r.lastExpire) >= remoteTelemetryExpireInterval {
		r.lastExpire = now
		registered := make(map[string]struct{})
		for _, agent := range ra.GetRegisteredAgents() {
			registered[remoteAgentIdentity(agent)] = struct{}{}
		}
		r.expire(registered, scraped, now)
	}
	r.emit(emit)
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

// newestRegistrationFilter accepts only the most recent registration of each agent name that serves telemetry. The
// name is the emitter label of the series, so two registrations with the same name would report the same series: that
// happens briefly when a remote agent re-registers or restarts, and the older one is the one that went away.
func (ra *remoteAgentRegistry) newestRegistrationFilter() func(remoteagentregistry.RegisteredAgent) bool {
	newest := make(map[string]string)
	orders := make(map[string]uint64)
	ra.agentMapMu.Lock()
	for _, agent := range ra.agentMap {
		if !slices.Contains(agent.services, TelemetryServiceName) {
			continue
		}
		name := agent.SanitizedDisplayName
		if agent.registrationOrder >= orders[name] {
			newest[name] = agent.SessionID
			orders[name] = agent.registrationOrder
		}
	}
	ra.agentMapMu.Unlock()
	return func(agent remoteagentregistry.RegisteredAgent) bool {
		return newest[agent.SanitizedDisplayName] == agent.SessionID
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
