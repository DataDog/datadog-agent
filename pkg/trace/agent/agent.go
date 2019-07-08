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
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const processStatsInterval = time.Minute

// Agent struct holds all the sub-routines structs and make the data flow between them
type Agent struct {
	Receiver           *api.HTTPReceiver
	Concentrator       *stats.Concentrator
	Blacklister        *filters.Blacklister
	Replacer           *filters.Replacer
	ScoreSampler       *Sampler
	ErrorsScoreSampler *Sampler
	PrioritySampler    *Sampler
	EventProcessor     *event.Processor
	TraceWriter        *writer.TraceWriter
	ServiceWriter      *writer.ServiceWriter
	StatsWriter        *writer.StatsWriter
	ServiceExtractor   *TraceServiceExtractor
	ServiceMapper      *ServiceMapper

	// obfuscator is used to obfuscate sensitive data from various span
	// tags based on their type.
	obfuscator *obfuscate.Obfuscator

	spansOut chan *writer.SampledSpans

	// config
	conf    *config.AgentConfig
	dynConf *sampler.DynamicConfig

	// Used to synchronize on a clean exit
	ctx context.Context
}

// NewAgent returns a new Agent object, ready to be started. It takes a context
// which may be cancelled in order to gracefully stop the agent.
func NewAgent(ctx context.Context, conf *config.AgentConfig) *Agent {
	dynConf := sampler.NewDynamicConfig(conf.DefaultEnv)

	// inter-component channels
	rawTraceChan := make(chan pb.Trace, 5000)
	spansOut := make(chan *writer.SampledSpans, 1000)
	statsChan := make(chan []stats.Bucket)
	serviceChan := make(chan pb.ServicesMetadata, 50)
	filteredServiceChan := make(chan pb.ServicesMetadata, 50)

	// create components
	r := api.NewHTTPReceiver(conf, dynConf, rawTraceChan, serviceChan)
	c := stats.NewConcentrator(
		conf.ExtraAggregators,
		conf.BucketInterval.Nanoseconds(),
		statsChan,
	)

	obf := obfuscate.NewObfuscator(conf.Obfuscation)
	ss := NewScoreSampler(conf)
	ess := NewErrorsSampler(conf)
	ps := NewPrioritySampler(conf, dynConf)
	ep := eventProcessorFromConf(conf)
	se := NewTraceServiceExtractor(serviceChan)
	sm := NewServiceMapper(serviceChan, filteredServiceChan)
	tw := writer.NewTraceWriter(conf, spansOut)
	sw := writer.NewStatsWriter(conf, statsChan)
	svcW := writer.NewServiceWriter(conf, filteredServiceChan)

	return &Agent{
		Receiver:           r,
		Concentrator:       c,
		Blacklister:        filters.NewBlacklister(conf.Ignore["resource"]),
		Replacer:           filters.NewReplacer(conf.ReplaceTags),
		ScoreSampler:       ss,
		ErrorsScoreSampler: ess,
		PrioritySampler:    ps,
		EventProcessor:     ep,
		TraceWriter:        tw,
		StatsWriter:        sw,
		ServiceWriter:      svcW,
		ServiceExtractor:   se,
		ServiceMapper:      sm,
		obfuscator:         obf,
		spansOut:           spansOut,
		conf:               conf,
		dynConf:            dynConf,
		ctx:                ctx,
	}
}

// Run starts routers routines and individual pieces then stop them when the exit order is received
func (a *Agent) Run() {
	for _, starter := range []interface{ Start() }{
		a.Receiver,
		a.ServiceMapper,
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
	go a.ServiceWriter.Run()

	for i := 0; i < runtime.NumCPU(); i++ {
		go a.work()
	}

	a.loop()
}

func (a *Agent) work() {
	for {
		select {
		case t, ok := <-a.Receiver.Out:
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
			a.ServiceMapper.Stop()
			a.ServiceWriter.Stop()
			a.ScoreSampler.Stop()
			a.ErrorsScoreSampler.Stop()
			a.PrioritySampler.Stop()
			a.EventProcessor.Stop()
			return
		}
	}
}

// Process is the default work unit that receives a trace, transforms it and
// passes it downstream.
func (a *Agent) Process(t pb.Trace) {
	if len(t) == 0 {
		log.Debugf("Skipping received empty trace")
		return
	}

	defer timing.Since("datadog.trace_agent.internal.process_trace_ms", time.Now())

	// Root span is used to carry some trace-level metadata, such as sampling rate and priority.
	root := traceutil.GetRoot(t)

	// We get the address of the struct holding the stats associated to no tags.
	// TODO: get the real tagStats related to this trace payload (per lang/version).
	ts := a.Receiver.Stats.GetTagStats(info.Tags{})

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
		atomic.AddInt64(&ts.SpansFiltered, int64(len(t)))
		return
	}

	// Extra sanitization steps of the trace.
	for _, span := range t {
		a.obfuscator.Obfuscate(span)
		Truncate(span)
	}
	a.Replacer.Replace(&t)

	// Extract the client sampling rate.
	clientSampleRate := sampler.GetGlobalRate(root)
	sampler.SetClientRate(root, clientSampleRate)
	// Combine it with the pre-sampling rate.
	rateLimiterRate := a.Receiver.RateLimiter.RealRate()
	sampler.SetPreSampleRate(root, rateLimiterRate)
	// Update root's global sample rate to include the presampler rate as well
	sampler.AddGlobalRate(root, rateLimiterRate)

	// Figure out the top-level spans and sublayers now as it involves modifying the Metrics map
	// which is not thread-safe while samplers and Concentrator might modify it too.
	traceutil.ComputeTopLevel(t)

	subtraces := stats.ExtractTopLevelSubtraces(t, root)
	sublayers := make(map[*pb.Span][]stats.SublayerValue)
	for _, subtrace := range subtraces {
		subtraceSublayers := stats.ComputeSublayers(subtrace.Trace)
		sublayers[subtrace.Root] = subtraceSublayers
		stats.SetSublayersOnSpan(subtrace.Root, subtraceSublayers)
	}

	pt := ProcessedTrace{
		Trace:         t,
		WeightedTrace: stats.NewWeightedTrace(t, root),
		Root:          root,
		Env:           a.conf.DefaultEnv,
		Sublayers:     sublayers,
	}
	// Replace Agent-configured environment with `env` coming from span tag.
	if tenv := traceutil.GetEnv(t); tenv != "" {
		pt.Env = tenv
	}

	go func() {
		defer watchdog.LogOnPanic()
		a.ServiceExtractor.Process(pt.WeightedTrace)
	}()

	go func(pt ProcessedTrace) {
		defer watchdog.LogOnPanic()
		defer timing.Since("datadog.trace_agent.internal.concentrator_ms", time.Now())
		// Everything is sent to concentrator for stats, regardless of sampling.
		a.Concentrator.Add(&stats.Input{
			Trace:     pt.WeightedTrace,
			Sublayers: pt.Sublayers,
			Env:       pt.Env,
		})
	}(pt)

	// Don't go through sampling for < 0 priority traces
	if priority < 0 {
		return
	}
	// Run both full trace sampling and transaction extraction in another goroutine.
	go func(pt ProcessedTrace) {
		defer watchdog.LogOnPanic()
		defer timing.Since("datadog.trace_agent.internal.sample_ms", time.Now())

		var sampledSpans writer.SampledSpans

		sampled, rate := a.sample(pt)
		if sampled {
			pt.Sampled = sampled
			sampler.AddGlobalRate(pt.Root, rate)
			sampledSpans.Trace = pt.Trace
		}

		// NOTE: Events can be processed on non-sampled traces.
		events, numExtracted := a.EventProcessor.Process(pt.Root, pt.Trace)
		sampledSpans.Events = events

		atomic.AddInt64(&ts.EventsExtracted, int64(numExtracted))
		atomic.AddInt64(&ts.EventsSampled, int64(len(sampledSpans.Events)))

		if !sampledSpans.Empty() {
			a.spansOut <- &sampledSpans
		}
	}(pt)
}

func (a *Agent) sample(pt ProcessedTrace) (sampled bool, rate float64) {
	var sampledPriority, sampledScore bool
	var ratePriority, rateScore float64

	if _, ok := pt.GetSamplingPriority(); ok {
		sampledPriority, ratePriority = a.PrioritySampler.Add(pt)
	}

	if traceContainsError(pt.Trace) {
		sampledScore, rateScore = a.ErrorsScoreSampler.Add(pt)
	} else {
		sampledScore, rateScore = a.ScoreSampler.Add(pt)
	}

	return sampledScore || sampledPriority, sampler.CombineRates(ratePriority, rateScore)
}

func traceContainsError(trace pb.Trace) bool {
	for _, span := range trace {
		if span.Error != 0 {
			return true
		}
	}
	return false
}

func eventProcessorFromConf(conf *config.AgentConfig) *event.Processor {
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
