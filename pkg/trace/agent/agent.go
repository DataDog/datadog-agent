// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/remoteconfighandler"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
)

const (
	// tagHostname specifies the hostname of the tracer.
	// DEPRECATED: Tracer hostname is now specified as a TracerPayload field.
	tagHostname = "_dd.hostname"

	// manualSampling is the value for _dd.p.dm when user sets sampling priority directly in code.
	manualSampling = "-4"

	// tagDecisionMaker specifies the sampling decision maker
	tagDecisionMaker = "_dd.p.dm"
)

// Agent struct holds all the sub-routines structs and make the data flow between them
type Agent struct {
	Receiver              *api.HTTPReceiver
	OTLPReceiver          *api.OTLPReceiver
	Concentrator          *stats.Concentrator
	ClientStatsAggregator *stats.ClientStatsAggregator
	Blacklister           *filters.Blacklister
	Replacer              *filters.Replacer
	PrioritySampler       *sampler.PrioritySampler
	ErrorsSampler         *sampler.ErrorsSampler
	RareSampler           *sampler.RareSampler
	NoPrioritySampler     *sampler.NoPrioritySampler
	EventProcessor        *event.Processor
	TraceWriter           *writer.TraceWriter
	StatsWriter           *writer.StatsWriter
	RemoteConfigHandler   *remoteconfighandler.RemoteConfigHandler

	// obfuscator is used to obfuscate sensitive data from various span
	// tags based on their type.
	obfuscator     *obfuscate.Obfuscator
	cardObfuscator *ccObfuscator

	// DiscardSpan will be called on all spans, if non-nil. If it returns true, the span will be deleted before processing.
	DiscardSpan func(*pb.Span) bool

	// ModifySpan will be called on all spans, if non-nil.
	ModifySpan func(*pb.Span)

	// In takes incoming payloads to be processed by the agent.
	In chan *api.Payload

	// config
	conf *config.AgentConfig

	// Used to synchronize on a clean exit
	ctx context.Context

	// tfs is the list of transformers used by agent.Process()
	transformers []Transformer
}

// NewAgent returns a new Agent object, ready to be started. It takes a context
// which may be cancelled in order to gracefully stop the agent.
func NewAgent(ctx context.Context, conf *config.AgentConfig) *Agent {
	dynConf := sampler.NewDynamicConfig()
	in := make(chan *api.Payload, 1000)
	statsChan := make(chan pb.StatsPayload, 100)

	oconf := conf.Obfuscation.Export()
	if oconf.Statsd == nil {
		oconf.Statsd = metrics.Client
	}
	agnt := &Agent{
		Concentrator:          stats.NewConcentrator(conf, statsChan, time.Now()),
		ClientStatsAggregator: stats.NewClientStatsAggregator(conf, statsChan),
		Blacklister:           filters.NewBlacklister(conf.Ignore["resource"]),
		Replacer:              filters.NewReplacer(conf.ReplaceTags),
		PrioritySampler:       sampler.NewPrioritySampler(conf, dynConf),
		ErrorsSampler:         sampler.NewErrorsSampler(conf),
		RareSampler:           sampler.NewRareSampler(conf),
		NoPrioritySampler:     sampler.NewNoPrioritySampler(conf),
		EventProcessor:        newEventProcessor(conf),
		TraceWriter:           writer.NewTraceWriter(conf),
		StatsWriter:           writer.NewStatsWriter(conf, statsChan),
		obfuscator:            obfuscate.NewObfuscator(oconf),
		cardObfuscator:        newCreditCardsObfuscator(conf.Obfuscation.CreditCards),
		In:                    in,
		conf:                  conf,
		ctx:                   ctx,
	}
	agnt.Receiver = api.NewHTTPReceiver(conf, dynConf, in, agnt)
	agnt.OTLPReceiver = api.NewOTLPReceiver(in, conf)
	agnt.RemoteConfigHandler = remoteconfighandler.New(conf, agnt.PrioritySampler, agnt.RareSampler, agnt.ErrorsSampler)
	agnt.transformers = agnt.makeTransformers()
	return agnt
}

// Run starts routers routines and individual pieces then stop them when the exit order is received.
func (a *Agent) Run() {
	for _, starter := range []interface{ Start() }{
		a.Receiver,
		a.Concentrator,
		a.ClientStatsAggregator,
		a.PrioritySampler,
		a.ErrorsSampler,
		a.NoPrioritySampler,
		a.EventProcessor,
		a.OTLPReceiver,
		a.RemoteConfigHandler,
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

// FlushSync flushes traces sychronously. This method only works when the agent is configured in synchronous flushing
// mode via the apm_config.sync_flush option.
func (a *Agent) FlushSync() {
	if !a.conf.SynchronousFlushing {
		log.Critical("(*Agent).FlushSync called without apm_conf.sync_flushing enabled. No data was sent to Datadog.")
		return
	}

	if err := a.StatsWriter.FlushSync(); err != nil {
		log.Errorf("Error flushing stats: %s", err.Error())
		return
	}
	if err := a.TraceWriter.FlushSync(); err != nil {
		log.Errorf("Error flushing traces: %s", err.Error())
		return
	}
}

func (a *Agent) work() {
	for {
		select {
		case p, ok := <-a.In:
			if !ok {
				return
			}
			a.Process(p)
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
			for _, stopper := range []interface{ Stop() }{
				a.Concentrator,
				a.ClientStatsAggregator,
				a.TraceWriter,
				a.StatsWriter,
				a.PrioritySampler,
				a.ErrorsSampler,
				a.NoPrioritySampler,
				a.RareSampler,
				a.EventProcessor,
				a.OTLPReceiver,
				a.obfuscator,
				a.obfuscator,
				a.cardObfuscator,
			} {
				stopper.Stop()
			}
			return
		}
	}
}

// setRootSpanTags sets up any necessary tags on the root span.
func (a *Agent) setRootSpanTags(_ *api.Metadata, pt *traceutil.ProcessedTrace) error {
	clientSampleRate := sampler.GetGlobalRate(pt.Root)
	sampler.SetClientRate(pt.Root, clientSampleRate)

	if ratelimiter := a.Receiver.RateLimiter; ratelimiter.Active() {
		rate := ratelimiter.RealRate()
		sampler.SetPreSampleRate(pt.Root, rate)
	}

	// TODO: add azure specific tags here (at least for now, so chill out and
	// just do it) "it doesn't have to be pretty it just has to work"
	if a.conf.InAzureAppServices {
		for k, v := range traceutil.GetAppServicesTags() {
			traceutil.SetMeta(pt.Root, k, v)
		}
	}
	return nil
}

func normalizePayloadEnv(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	pt.TracerEnv = traceutil.NormalizeTag(pt.TracerEnv)
	return nil
}

func (a *Agent) discardSpans(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	if a.DiscardSpan == nil {
		return nil
	}
	n := 0
	for _, span := range pt.TraceChunk.Spans {
		if !a.DiscardSpan(span) {
			pt.TraceChunk.Spans[n] = span
			n++
		}
	}
	// set everything at the back of the array to nil to avoid memory leaking
	// since we're going to have garbage elements at the back of the slice.
	for i := n; i < len(pt.TraceChunk.Spans); i++ {
		pt.TraceChunk.Spans[i] = nil
	}
	pt.TraceChunk.Spans = pt.TraceChunk.Spans[:n]
	return nil
}

func rejectEmpty(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	if len(pt.TraceChunk.Spans) == 0 {
		return errors.New("Skipping received empty trace")
	}
	return nil
}

func countSpansReceived(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	m.Source.SpansReceived.Add(int64(len(pt.TraceChunk.Spans)))
	return nil
}

func (a *Agent) blocklist(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	if !a.Blacklister.Allows(pt.Root) {
		m.Source.TracesFiltered.Inc()
		m.Source.SpansFiltered.Add(int64(len(pt.TraceChunk.Spans)))
		return fmt.Errorf("Trace rejected by ignore resources rules. root: %v", pt.Root)
	}
	return nil
}

func (a *Agent) tagFilter(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	if filteredByTags(pt.Root, a.conf.RequireTags, a.conf.RejectTags) {
		m.Source.TracesFiltered.Inc()
		m.Source.SpansFiltered.Add(int64(len(pt.TraceChunk.Spans)))
		return fmt.Errorf("Trace rejected as it fails to meet tag requirements. root: %v", pt.Root)
	}
	return nil
}

func eachSpan(fs ...SpanTransformer) Transformer {
	return func(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
		for _, span := range pt.TraceChunk.Spans {
			for _, f := range fs {
				if err := f(m, pt, span); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func (a *Agent) applyGlobalTags(m *api.Metadata, pt *traceutil.ProcessedTrace, s *pb.Span) error {
	for k, v := range a.conf.GlobalTags {
		if k == tagOrigin {
			pt.TraceChunk.Origin = v
		} else {
			traceutil.SetMeta(s, k, v)
		}
	}
	return nil
}

func (a *Agent) modifySpan(m *api.Metadata, pt *traceutil.ProcessedTrace, s *pb.Span) error {
	if a.ModifySpan != nil {
		a.ModifySpan(s)
	}
	return nil
}

func updateTopLevel(m *api.Metadata, pt *traceutil.ProcessedTrace, s *pb.Span) error {
	if m.ClientComputedTopLevel {
		traceutil.UpdateTracerTopLevel(s)
	}
	return nil
}

func computeTopLevel(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	if !m.ClientComputedTopLevel {
		// Figure out the top-level spans now as it involves modifying the Metrics map
		// which is not thread-safe while samplers and Concentrator might modify it too.
		traceutil.ComputeTopLevel(pt.TraceChunk.Spans)
	}
	return nil
}

func (a *Agent) makeTransformers() []Transformer {
	return []Transformer{
		normalizePayloadEnv,
		a.discardSpans,
		rejectEmpty,
		countSpansReceived,
		normalizeChunk,
		normalizeSpans,
		a.blocklist,
		a.tagFilter,
		eachSpan(
			a.applyGlobalTags,
			a.modifySpan,
			a.obfuscateSpan,
			Truncate,
			updateTopLevel,
			a.Replacer.Replace,
		),
		a.setRootSpanTags,
		computeTopLevel,
	}
}

// Transformer is a transformation of a processed trace. If a Transformer returns an error, the
// ProcessedTrace will be discarded and not sent to the intake.
type Transformer func(*api.Metadata, *traceutil.ProcessedTrace) error

// SpanTransformer is a transformation of a span within a trace. If a SpanTransformer returns an
// error, the ProcessedTrace will be discarded and not sent to the intake.
type SpanTransformer func(*api.Metadata, *traceutil.ProcessedTrace, *pb.Span) error

// transform applies all of the transformations in a.transformers
func (a *Agent) transform(m *api.Metadata, pt *traceutil.ProcessedTrace) error {
	for _, t := range a.transformers {
		err := t(m, pt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) Process(p *api.Payload) {
	if len(p.TracerPayload.Chunks) == 0 {
		log.Debugf("Skipping received empty payload")
		return
	}
	now := time.Now()
	defer timing.Since("datadog.trace_agent.internal.process_payload_ms", now)
	ts := p.Meta.Source
	ss := new(writer.SampledChunks)
	statsInput := stats.NewStatsInput(len(p.TracerPayload.Chunks), p.TracerPayload.ContainerID, p.Meta.ClientComputedStats, a.conf)

	for i := 0; i < len(p.TracerPayload.Chunks); {
		chunk := p.TracerPayload.Chunks[i]
		// Remove nil spans from the chunk. The rest of the code can assume a span is not nil.
		chunk.Spans = filterNil(chunk.Spans)
		root := traceutil.GetRoot(chunk.Spans)
		if p.TracerPayload.Hostname == "" {
			// Older tracers set tracer hostname in the root span.
			p.TracerPayload.Hostname = root.Meta[tagHostname]
		}
		if p.TracerPayload.Env == "" {
			p.TracerPayload.Env = traceutil.GetEnv(root, chunk)
		}
		if p.TracerPayload.AppVersion == "" {
			p.TracerPayload.AppVersion = traceutil.GetAppVersion(root, chunk)
		}
		pt := traceutil.ProcessedTrace{
			TraceChunk:             chunk,
			Root:                   root,
			AppVersion:             p.TracerPayload.AppVersion,
			TracerEnv:              p.TracerPayload.Env,
			TracerHostname:         p.TracerPayload.Hostname,
			ClientDroppedP0sWeight: float64(p.Meta.ClientDroppedP0s) / float64(len(p.TracerPayload.Chunks)),
		}

		// Transform the chunk
		if err := a.transform(&p.Meta, &pt); err != nil {
			log.Debugf("Removing Chunk: %v", err)
			p.TracerPayload.RemoveChunk(i)
			continue
		}

		if !p.Meta.ClientComputedStats {
			statsInput.Traces = append(statsInput.Traces, pt)
		}

		// Sampling
		numEvents, keep, filteredChunk := a.sample(now, ts, pt)
		if !keep {
			keep = sampler.ApplySpanSampling(chunk)
		}
		if !keep {
			if numEvents == 0 {
				// the trace was dropped and no analyzed span were kept
				p.TracerPayload.RemoveChunk(i)
				continue
			}
			// The sampler step filtered a subset of spans in the chunk. The new
			// filtered chunk is added to the TracerPayload to be sent to
			// TraceWriter. The complete chunk is still sent to the stats
			// concentrator.
			p.TracerPayload.Chunks[i] = filteredChunk
		}

		if !chunk.DroppedTrace {
			ss.SpanCount += int64(len(chunk.Spans))
		}
		ss.EventCount += numEvents
		ss.Size += chunk.Msgsize()
		i++

		if ss.Size > writer.MaxPayloadSize {
			// payload size is getting big; split and flush what we have so far
			ss.TracerPayload = p.TracerPayload.Cut(i)
			i = 0
			ss.TracerPayload.Chunks = newChunksArray(ss.TracerPayload.Chunks)
			a.TraceWriter.In <- ss
			ss = new(writer.SampledChunks)
		}
	}
	ss.TracerPayload = p.TracerPayload
	ss.TracerPayload.Chunks = newChunksArray(p.TracerPayload.Chunks)
	if ss.Size > 0 {
		a.TraceWriter.In <- ss
	}
	if len(statsInput.Traces) > 0 {
		a.Concentrator.In <- statsInput
	}
}

// newChunksArray creates a new array which will point only to sampled chunks.

// The underlying array behind TracePayload.Chunks points to unsampled chunks
// preventing them from being collected by the GC.
func newChunksArray(chunks []*pb.TraceChunk) []*pb.TraceChunk {
	new := make([]*pb.TraceChunk, len(chunks))
	copy(new, chunks)
	return new
}

var _ api.StatsProcessor = (*Agent)(nil)

func (a *Agent) processStats(in pb.ClientStatsPayload, lang, tracerVersion string) pb.ClientStatsPayload {
	enableContainers := features.Has("enable_cid_stats") || (a.conf.FargateOrchestrator != config.OrchestratorUnknown)
	if !enableContainers || features.Has("disable_cid_stats") {
		// only allow the ContainerID stats dimension if we're in a Fargate instance or it's
		// been explicitly enabled and it's not prohibited by the disable_cid_stats feature flag.
		in.ContainerID = ""
		in.Tags = nil
	}
	if in.Env == "" {
		in.Env = a.conf.DefaultEnv
	}
	in.Env = traceutil.NormalizeTag(in.Env)
	if in.TracerVersion == "" {
		in.TracerVersion = tracerVersion
	}
	if in.Lang == "" {
		in.Lang = lang
	}
	for i, group := range in.Stats {
		n := 0
		for _, b := range group.Stats {
			normalizeStatsGroup(&b, lang)
			if !a.Blacklister.AllowsStat(&b) {
				continue
			}
			a.obfuscateStatsGroup(&b)
			a.Replacer.ReplaceStatsGroup(&b)
			group.Stats[n] = b
			n++
		}
		in.Stats[i].Stats = group.Stats[:n]
		mergeDuplicates(in.Stats[i])
	}
	return in
}

func mergeDuplicates(s pb.ClientStatsBucket) {
	indexes := make(map[stats.Aggregation]int, len(s.Stats))
	for i, g := range s.Stats {
		a := stats.NewAggregationFromGroup(g)
		if j, ok := indexes[a]; ok {
			s.Stats[j].Hits += g.Hits
			s.Stats[j].Errors += g.Errors
			s.Stats[j].Duration += g.Duration
			s.Stats[i].Hits = 0
			s.Stats[i].Errors = 0
			s.Stats[i].Duration = 0
		} else {
			indexes[a] = i
		}
	}
}

// ProcessStats processes incoming client stats in from the given tracer.
func (a *Agent) ProcessStats(in pb.ClientStatsPayload, lang, tracerVersion string) {
	a.ClientStatsAggregator.In <- a.processStats(in, lang, tracerVersion)
}

func isManualUserDrop(priority sampler.SamplingPriority, pt traceutil.ProcessedTrace) bool {
	if priority != sampler.PriorityUserDrop {
		return false
	}
	dm, hasDm := pt.Root.Meta[tagDecisionMaker]
	if !hasDm {
		return false
	}
	return dm == manualSampling
}

// sample reports the number of events found in pt and whether the chunk should be kept as a trace.
func (a *Agent) sample(now time.Time, ts *info.TagStats, pt traceutil.ProcessedTrace) (numEvents int64, keep bool, filteredChunk *pb.TraceChunk) {
	priority, hasPriority := sampler.GetSamplingPriority(pt.TraceChunk)

	if hasPriority {
		ts.TracesPerSamplingPriority.CountSamplingPriority(priority)
	} else {
		ts.TracesPriorityNone.Inc()
	}

	if features.Has("error_rare_sample_tracer_drop") {
		if isManualUserDrop(priority, pt) {
			return 0, false, nil
		}
	} else { // This path to be deleted once manualUserDrop detection is available on all tracers for P < 1.
		if priority < 0 {
			return 0, false, nil
		}
	}

	sampled := a.runSamplers(now, pt, hasPriority)

	filteredChunk = pt.TraceChunk
	if !sampled {
		filteredChunk = new(pb.TraceChunk)
		*filteredChunk = *pt.TraceChunk
		filteredChunk.DroppedTrace = true
	}
	numEvents, numExtracted := a.EventProcessor.Process(pt.Root, filteredChunk)

	ts.EventsExtracted.Add(numExtracted)
	ts.EventsSampled.Add(numEvents)

	return numEvents, sampled, filteredChunk
}

// runSamplers runs all the agent's samplers on pt and returns the sampling decision
// along with the sampling rate.
func (a *Agent) runSamplers(now time.Time, pt traceutil.ProcessedTrace, hasPriority bool) bool {
	if hasPriority {
		return a.samplePriorityTrace(now, pt)
	}
	return a.sampleNoPriorityTrace(now, pt)
}

// samplePriorityTrace samples traces with priority set on them. PrioritySampler and
// ErrorSampler are run in parallel. The RareSampler catches traces with rare top-level
// or measured spans that are not caught by PrioritySampler and ErrorSampler.
func (a *Agent) samplePriorityTrace(now time.Time, pt traceutil.ProcessedTrace) bool {
	// run this early to make sure the signature gets counted by the RareSampler.
	rare := a.RareSampler.Sample(now, pt.TraceChunk, pt.TracerEnv)
	if a.PrioritySampler.Sample(now, pt.TraceChunk, pt.Root, pt.TracerEnv, pt.ClientDroppedP0sWeight) {
		return true
	}
	if traceContainsError(pt.TraceChunk.Spans) {
		return a.ErrorsSampler.Sample(now, pt.TraceChunk.Spans, pt.Root, pt.TracerEnv)
	}
	return rare
}

// sampleNoPriorityTrace samples traces with no priority set on them. The traces
// get sampled by either the score sampler or the error sampler if they have an error.
func (a *Agent) sampleNoPriorityTrace(now time.Time, pt traceutil.ProcessedTrace) bool {
	if traceContainsError(pt.TraceChunk.Spans) {
		return a.ErrorsSampler.Sample(now, pt.TraceChunk.Spans, pt.Root, pt.TracerEnv)
	}
	return a.NoPrioritySampler.Sample(now, pt.TraceChunk.Spans, pt.Root, pt.TracerEnv)
}

func traceContainsError(trace pb.Trace) bool {
	for _, span := range trace {
		if span.Error != 0 {
			return true
		}
	}
	return false
}

func filteredByTags(root *pb.Span, require, reject []*config.Tag) bool {
	for _, tag := range reject {
		if v, ok := root.Meta[tag.K]; ok && (tag.V == "" || v == tag.V) {
			return true
		}
	}
	for _, tag := range require {
		v, ok := root.Meta[tag.K]
		if !ok || (tag.V != "" && v != tag.V) {
			return true
		}
	}
	return false
}

func newEventProcessor(conf *config.AgentConfig) *event.Processor {
	extractors := []event.Extractor{event.NewMetricBasedExtractor()}
	if len(conf.AnalyzedSpansByService) > 0 {
		extractors = append(extractors, event.NewFixedRateExtractor(conf.AnalyzedSpansByService))
	} else if len(conf.AnalyzedRateByServiceLegacy) > 0 {
		extractors = append(extractors, event.NewLegacyExtractor(conf.AnalyzedRateByServiceLegacy))
	}

	return event.NewProcessor(extractors, conf.MaxEPS)
}

// SetGlobalTagsUnsafe sets global tags to the agent configuration. Unsafe for concurrent use.
func (a *Agent) SetGlobalTagsUnsafe(tags map[string]string) {
	a.conf.GlobalTags = tags
}
