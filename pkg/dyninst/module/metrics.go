// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package module

import (
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// metricProbeNamePrefix is the prefix of the names of the metrics reported by
// metric probes. The backend relies on it, and all tracers use it.
const metricProbeNamePrefix = "dynamic.instrumentation.metric.probe."

// defaultMetricsReportInterval is how often the state of metric probes is read
// and reported.
const defaultMetricsReportInterval = 2 * time.Second

// metricsClient is the part of the DogStatsD client used to report the values
// of metric probes.
type metricsClient interface {
	Count(name string, value int64, tags []string, rate float64) error
	Gauge(name string, value float64, tags []string, rate float64) error
}

// metricsReporter periodically reads the state of the metric probes of the
// loaded programs, as recorded by the BPF programs, and reports it to
// DogStatsD.
type metricsReporter struct {
	client metricsClient
	// onEmitting is called the first time a probe's metric is reported, with
	// mu held.
	onEmitting func(ir.ProgramID, ir.ProbeDefinition)

	mu       sync.Mutex
	programs map[ir.ProgramID]*metricsProgram

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// metricsProgram holds the metric probes of one loaded program.
type metricsProgram struct {
	// readStates reads the probes' state from the program's metrics_buf.
	readStates func() []loader.MetricState
	probes     []*reportedMetricProbe
	// last is the state read by the previous report, indexed by probe ID.
	last []loader.MetricState
}

type reportedMetricProbe struct {
	probeID uint32
	def     ir.ProbeDefinition
	kind    ir.MetricKind
	name    string
	tags    []string
	// gaugeValue converts the bits of a gauge probe's value, as recorded by
	// the BPF program, to the reported value.
	gaugeValue func(bits uint64) float64
	emitting   bool
}

var metricsReportErrorLogLimiter = rate.NewLimiter(rate.Every(time.Minute), 10)

// newMetricsReporter creates a metrics reporter that reports every interval
// until close is called.
func newMetricsReporter(
	client metricsClient,
	interval time.Duration,
	onEmitting func(ir.ProgramID, ir.ProbeDefinition),
) *metricsReporter {
	r := &metricsReporter{
		client:     client,
		onEmitting: onEmitting,
		programs:   make(map[ir.ProgramID]*metricsProgram),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go r.run(interval)
	return r
}

func (r *metricsReporter) run(interval time.Duration) {
	defer close(r.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.reportAll()
		}
	}
}

// close stops the periodic reports. Programs that are still registered are
// not reported again.
func (r *metricsReporter) close() {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
}

// register starts reporting the metric probes of a program. It does nothing if
// the program has no metric probes. readStates reads the program's metric
// probe state; it is called until unregister returns.
func (r *metricsReporter) register(
	programID ir.ProgramID,
	program *ir.Program,
	readStates func() []loader.MetricState,
	runtimeID procRuntimeID,
) {
	var probes []*reportedMetricProbe
	for probeID, probe := range program.Probes {
		mp, ok := probe.ProbeDefinition.(ir.MetricProbeDefinition)
		if !ok || probe.GetKind() != ir.ProbeKindMetric {
			continue
		}
		probes = append(probes, &reportedMetricProbe{
			probeID:    uint32(probeID),
			def:        probe.ProbeDefinition,
			kind:       mp.GetMetricKind(),
			name:       metricProbeNamePrefix + sanitizeMetricName(mp.GetMetricName()),
			tags:       metricProbeTags(probe.ProbeDefinition, runtimeID),
			gaugeValue: gaugeValueDecoder(probe),
		})
	}
	if len(probes) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.programs[programID] = &metricsProgram{readStates: readStates, probes: probes}
}

// report reports the metric probes of a program that were hit since the
// previous report. The runtime calls it when the program is detached, so that
// the last hits aren't lost and the probes can still be marked as emitting.
func (r *metricsReporter) report(programID ir.ProgramID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.programs[programID]; ok {
		r.notifyEmitting(programID, r.reportLocked(p))
	}
}

// unregister stops reporting the metric probes of a program. It must be called
// before the program's BPF maps are closed. Hits since the previous report
// aren't reported; call report first.
func (r *metricsReporter) unregister(programID ir.ProgramID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.programs, programID)
}

func (r *metricsReporter) reportAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for programID, p := range r.programs {
		r.notifyEmitting(programID, r.reportLocked(p))
	}
}

// reportLocked reports the probes of a program that were hit since the
// previous report. It returns the probes that were reported for the first
// time.
func (r *metricsReporter) reportLocked(p *metricsProgram) (newlyEmitting []*reportedMetricProbe) {
	states := p.readStates()
	if len(p.last) < len(states) {
		p.last = append(p.last, make([]loader.MetricState, len(states)-len(p.last))...)
	}
	for _, probe := range p.probes {
		if int(probe.probeID) >= len(states) {
			continue
		}
		cur, prev := states[probe.probeID], p.last[probe.probeID]
		p.last[probe.probeID] = cur
		// The BPF program updates the sum or value and the update count
		// separately, so a read can see one change without the other. A probe
		// is reported when either changed, so that a change of its sum is
		// never saved without being sent.
		changed := cur.Updates != prev.Updates
		switch probe.kind {
		case ir.MetricKindCount:
			changed = changed || cur.Sum != prev.Sum
		case ir.MetricKindGauge:
			changed = changed || cur.Value != prev.Value
		}
		if !changed {
			continue
		}
		var err error
		switch probe.kind {
		case ir.MetricKindCount:
			err = r.client.Count(probe.name, cur.Sum-prev.Sum, probe.tags, 1)
		case ir.MetricKindGauge:
			err = r.client.Gauge(probe.name, probe.gaugeValue(cur.Value), probe.tags, 1)
		}
		if err != nil && metricsReportErrorLogLimiter.Allow() {
			log.Warnf("failed to report metric %s of probe %s: %v", probe.name, probe.def.GetID(), err)
		}
		if !probe.emitting {
			probe.emitting = true
			newlyEmitting = append(newlyEmitting, probe)
		}
	}
	return newlyEmitting
}

// notifyEmitting calls onEmitting for the probes of a program that a report
// marked as emitting. It must be called with r.mu held, in the same critical
// section as that report: the runtime unlinks a program from its process right
// after the program's last report, onEmitting does nothing for an unlinked
// program, and the probes are never notified again.
func (r *metricsReporter) notifyEmitting(programID ir.ProgramID, probes []*reportedMetricProbe) {
	if r.onEmitting == nil {
		return
	}
	for _, probe := range probes {
		r.onEmitting(programID, probe.def)
	}
}

// metricProbeTags returns the tags of the metrics of a metric probe: the
// probe's own tags, the probe ID, and the service, environment and version of
// the target process.
func metricProbeTags(def ir.ProbeDefinition, runtimeID procRuntimeID) []string {
	tags := make([]string, 0, len(def.GetTags())+4)
	for _, tag := range def.GetTags() {
		tags = append(tags, sanitizeMetricTag(tag))
	}
	tags = append(tags, "debugger.probeid:"+sanitizeMetricTag(def.GetID()))
	for _, t := range []struct{ key, value string }{
		{"service", runtimeID.service},
		{"env", runtimeID.environment},
		{"version", runtimeID.version},
	} {
		if t.value != "" {
			tags = append(tags, t.key+":"+sanitizeMetricTag(t.value))
		}
	}
	return tags
}

// gaugeValueDecoder returns the function that converts the bits of a gauge
// probe's value, as recorded by the BPF program, to the reported value. The
// BPF program sign-extends signed integers to 64 bits and records floats as
// their raw bits.
//
// All the instances of a probe record their values in the same metric state,
// so the type of the first instance's value is used for all of them.
func gaugeValueDecoder(probe *ir.Probe) func(bits uint64) float64 {
	var goKind reflect.Kind
	if len(probe.Instances) > 0 && len(probe.Instances[0].Events) > 0 {
		if rootType := probe.Instances[0].Events[0].Type; rootType != nil &&
			len(rootType.Expressions) > 0 {
			goKind, _ = rootType.Expressions[0].Expression.Type.GetGoKind()
		}
	}
	switch goKind {
	case reflect.Float32:
		return func(bits uint64) float64 { return float64(math.Float32frombits(uint32(bits))) }
	case reflect.Float64:
		return math.Float64frombits
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(bits uint64) float64 { return float64(int64(bits)) }
	default:
		return func(bits uint64) float64 { return float64(bits) }
	}
}

// sanitizeMetricName replaces the characters that don't belong in metric
// names with underscores. That includes all the characters that have a meaning
// in DogStatsD datagrams.
func sanitizeMetricName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
}

// sanitizeMetricTag replaces the characters that have a meaning in DogStatsD
// datagrams with underscores.
func sanitizeMetricTag(tag string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '|', ',', '\n', '\r':
			return '_'
		default:
			return r
		}
	}, tag)
}
