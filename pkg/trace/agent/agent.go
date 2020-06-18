// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tagContainersTags specifies the name of the tag which holds key/value
// pairs representing information about the container (Docker, EC2, etc).
const tagContainersTags = "_dd.tags.container"

// Agent struct holds all the sub-routines structs and make the data flow between them
type Agent struct {
	Receiver           *api.HTTPReceiver
	Concentrator       *stats.Concentrator
	Blacklister        *filters.Blacklister
	Replacer           *filters.Replacer
	ScoreSampler       *Sampler
	ErrorsScoreSampler *Sampler
	ExceptionSampler   *sampler.ExceptionSampler
	PrioritySampler    *Sampler
	EventProcessor     *event.Processor
	TraceWriter        *writer.TraceWriter
	StatsWriter        *writer.StatsWriter

	// obfuscator is used to obfuscate sensitive data from various span
	// tags based on their type.
	obfuscator *obfuscate.Obfuscator

	In  chan *api.Trace
	Out chan *writer.SampledSpans

	// config
	conf *config.AgentConfig

	// Used to synchronize on a clean exit
	ctx context.Context
}

// NewAgent returns a new Agent object, ready to be started. It takes a context
// which may be cancelled in order to gracefully stop the agent.
func NewAgent(ctx context.Context, conf *config.AgentConfig) *Agent {
	dynConf := sampler.NewDynamicConfig(conf.DefaultEnv)
	in := make(chan *api.Trace, 5000)
	out := make(chan *writer.SampledSpans, 1000)
	statsChan := make(chan []stats.Bucket)

	return &Agent{
		Receiver:           api.NewHTTPReceiver(conf, dynConf, in),
		Concentrator:       stats.NewConcentrator(conf.ExtraAggregators, conf.BucketInterval.Nanoseconds(), statsChan),
		Blacklister:        filters.NewBlacklister(conf.Ignore["resource"]),
		Replacer:           filters.NewReplacer(conf.ReplaceTags),
		ScoreSampler:       NewScoreSampler(conf),
		ExceptionSampler:   sampler.NewExceptionSampler(),
		ErrorsScoreSampler: NewErrorsSampler(conf),
		PrioritySampler:    NewPrioritySampler(conf, dynConf),
		EventProcessor:     newEventProcessor(conf),
		TraceWriter:        writer.NewTraceWriter(conf, out),
		StatsWriter:        writer.NewStatsWriter(conf, statsChan),
		obfuscator:         obfuscate.NewObfuscator(conf.Obfuscation),
		In:                 in,
		Out:                out,
		conf:               conf,
		ctx:                ctx,
	}
}

// Run starts routers routines and individual pieces then stop them when the exit order is received
func (a *Agent) Run() {
	for _, starter := range []interface{ Start() }{
		a.Receiver,
		a.Concentrator,
		a.ScoreSampler,
		a.ErrorsScoreSampler,
		a.PrioritySampler,
		a.EventProcessor,
	} {
		starter.Start()
	}

	go a.TraceWriter.Run()
	go a.StatsWriter.Run()

	for i := 0; i < runtime.NumCPU(); i++ {
		go a.work()
	}

	a.loop()
}

func (a *Agent) work() {
	for {
		select {
		case t, ok := <-a.In:
			if !ok {
				return
			}
			a.Process(t)
		}
	}

}

func (a *Agent) loop() {
	for {
		select {
		case <-a.ctx.Done():
			log.Info("Exiting...")
			if err := a.Receiver.Stop(); err != nil {
				log.Error(err)
			}
			a.Concentrator.Stop()
			a.TraceWriter.Stop()
			a.StatsWriter.Stop()
			a.ScoreSampler.Stop()
			a.ExceptionSampler.Stop()
			a.ErrorsScoreSampler.Stop()
			a.PrioritySampler.Stop()
			a.EventProcessor.Stop()
			return
		}
	}
}

// Process is the default work unit that receives a trace, transforms it and
// passes it downstream.
func (a *Agent) Process(t *api.Trace) {
	if len(t.Spans) == 0 {
		log.Debugf("Skipping received empty trace")
		return
	}

	defer timing.Since("datadog.trace_agent.internal.process_trace_ms", time.Now())

	// Root span is used to carry some trace-level metadata, such as sampling rate and priority.
	root := traceutil.GetRoot(t.Spans)

	// We get the address of the struct holding the stats associated to no tags.
	ts := a.Receiver.Stats.GetTagStats(*t.Source)

	// Extract priority early, as later goroutines might manipulate the Metrics map in parallel which isn't safe.
	priority, hasPriority := sampler.GetSamplingPriority(root)

	// Depending on the sampling priority, count that trace differently.
	stat := &ts.TracesPriorityNone
	if hasPriority {
		if priority < 0 {
			stat = &ts.TracesPriorityNeg
		} else if priority == 0 {
			stat = &ts.TracesPriority0
		} else if priority == 1 {
			stat = &ts.TracesPriority1
		} else {
			stat = &ts.TracesPriority2
		}
	}
	atomic.AddInt64(stat, 1)

	if !a.Blacklister.Allows(root) {
		log.Debugf("Trace rejected by blacklister. root: %v", root)
		atomic.AddInt64(&ts.TracesFiltered, 1)
		atomic.AddInt64(&ts.SpansFiltered, int64(len(t.Spans)))
		return
	}

	// Extra sanitization steps of the trace.
	for _, span := range t.Spans {
		a.obfuscator.Obfuscate(span)
		Truncate(span)
	}
	a.Replacer.Replace(t.Spans)

	{
		// this section sets up any necessary tags on the root:
		clientSampleRate := sampler.GetGlobalRate(root)
		sampler.SetClientRate(root, clientSampleRate)

		if ratelimiter := a.Receiver.RateLimiter; ratelimiter.Active() {
			rate := ratelimiter.RealRate()
			sampler.SetPreSampleRate(root, rate)
			sampler.AddGlobalRate(root, rate)
		}
		if t.ContainerTags != "" {
			traceutil.SetMeta(root, tagContainersTags, t.ContainerTags)
		}
	}

	// Figure out the top-level spans and sublayers now as it involves modifying the Metrics map
	// which is not thread-safe while samplers and Concentrator might modify it too.
	traceutil.ComputeTopLevel(t.Spans)

	subtraces := stats.ExtractSubtraces(t.Spans, root)
	sublayers := make(map[*pb.Span][]stats.SublayerValue)
	for _, subtrace := range subtraces {
		subtraceSublayers := stats.ComputeSublayers(subtrace.Trace)
		sublayers[subtrace.Root] = subtraceSublayers
		stats.SetSublayersOnSpan(subtrace.Root, subtraceSublayers)
	}

	pt := ProcessedTrace{
		Trace:         t.Spans,
		WeightedTrace: stats.NewWeightedTrace(t.Spans, root),
		Root:          root,
		Env:           a.conf.DefaultEnv,
		Sublayers:     sublayers,
	}
	if tenv := traceutil.GetEnv(t.Spans); tenv != "" {
		// this trace has a user defined env.
		pt.Env = tenv
	}

	if priority >= 0 {
		a.sample(ts, pt)
	}

	a.Concentrator.In <- &stats.Input{
		Trace:     pt.WeightedTrace,
		Sublayers: pt.Sublayers,
		Env:       pt.Env,
	}
}

// sample decides whether the trace will be kept and extracts any APM events
// from it.
func (a *Agent) sample(ts *info.TagStats, pt ProcessedTrace) {
	var ss writer.SampledSpans

	sampled, rate := a.runSamplers(pt)
	if sampled {
		sampler.AddGlobalRate(pt.Root, rate)
		ss.Trace = pt.Trace
	}

	events, numExtracted := a.EventProcessor.Process(pt.Root, pt.Trace)
	ss.Events = events

	atomic.AddInt64(&ts.EventsExtracted, int64(numExtracted))
	atomic.AddInt64(&ts.EventsSampled, int64(len(events)))

	if !ss.Empty() {
		a.Out <- &ss
	}
}

// runSamplers runs all the agent's samplers on pt and returns the sampling decision
// along with the sampling rate.
func (a *Agent) runSamplers(pt ProcessedTrace) (bool, float64) {
	if _, ok := pt.GetSamplingPriority(); ok {
		return a.samplePriorityTrace(pt)
	}
	return a.sampleNoPriorityTrace(pt)
}

// samplePriorityTrace samples traces with priority set on them. PrioritySampler and
// ErrorSampler are run in parallel. The ExceptionSampler catches traces with rare top-level
// or measured spans that are not caught by PrioritySampler and ErrorSampler.
func (a *Agent) samplePriorityTrace(pt ProcessedTrace) (sampled bool, rate float64) {
	sampledPriority, ratePriority := a.PrioritySampler.Add(pt)
	if traceContainsError(pt.Trace) {
		sampledError, rateError := a.ErrorsScoreSampler.Add(pt)
		return sampledError || sampledPriority, sampler.CombineRates(ratePriority, rateError)
	}
	if sampled := a.ExceptionSampler.Add(pt.Env, pt.Root, pt.Trace); sampled {
		return sampled, 1
	}
	return sampledPriority, ratePriority
}

// sampleNoPriorityTrace samples traces with no priority set on them. The traces
// get sampled by either the score sampler or the error sampler if they have an error.
func (a *Agent) sampleNoPriorityTrace(pt ProcessedTrace) (sampled bool, rate float64) {
	if traceContainsError(pt.Trace) {
		return a.ErrorsScoreSampler.Add(pt)
	}
	return a.ScoreSampler.Add(pt)
}

func traceContainsError(trace pb.Trace) bool {
	for _, span := range trace {
		if span.Error != 0 {
			return true
		}
	}
	return false
}

func newEventProcessor(conf *config.AgentConfig) *event.Processor {
	extractors := []event.Extractor{
		event.NewMetricBasedExtractor(),
	}
	if len(conf.AnalyzedSpansByService) > 0 {
		extractors = append(extractors, event.NewFixedRateExtractor(conf.AnalyzedSpansByService))
	} else if len(conf.AnalyzedRateByServiceLegacy) > 0 {
		extractors = append(extractors, event.NewLegacyExtractor(conf.AnalyzedRateByServiceLegacy))
	}

	return event.NewProcessor(extractors, conf.MaxEPS)
}
