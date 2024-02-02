// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

//
//nolint:revive // TODO(APM) Fix revive linter
package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"sync"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
)

// Agent struct holds all the sub-routines structs and make the data flow between them
type ServerlessAgent struct {
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
	TelemetryCollector    telemetry.TelemetryCollector
	DebugServer           *api.DebugServer

	// obfuscator is used to obfuscate sensitive data from various span
	// tags based on their type.
	obfuscator     *obfuscate.Obfuscator
	cardObfuscator *ccObfuscator

	// DiscardSpan will be called on all spans, if non-nil. If it returns true, the span will be deleted before processing.
	DiscardSpan func(*pb.Span) bool

	// ModifySpan will be called on all non-nil spans of received trace chunks.
	// Note that any modification of the trace chunk could be overwritten by subsequent ModifySpan calls.
	ModifySpan func(*pb.TraceChunk, *pb.Span)

	// In takes incoming payloads to be processed by the agent.
	In chan *api.Payload

	// config
	conf *config.AgentConfig

	// Used to synchronize on a clean exit
	ctx context.Context

	firstSpanMap sync.Map
}

// NewAgent returns a new Agent object, ready to be started. It takes a context
// which may be cancelled in order to gracefully stop the agent.
func NewServerlessAgent(ctx context.Context, conf *config.AgentConfig, telemetryCollector telemetry.TelemetryCollector) *ServerlessAgent {
	dynConf := sampler.NewDynamicConfig()
	log.Infof("Starting Agent with processor trace buffer of size %d", conf.TraceBuffer)
	in := make(chan *api.Payload, conf.TraceBuffer)
	statsChan := make(chan *pb.StatsPayload, 1)
	oconf := conf.Obfuscation.Export(conf)
	if oconf.Statsd == nil {
		oconf.Statsd = metrics.Client
	}
	agnt := &ServerlessAgent{
		Concentrator:          stats.NewConcentrator(conf, statsChan, time.Now()),
		ClientStatsAggregator: stats.NewClientStatsAggregator(conf, statsChan),
		Blacklister:           filters.NewBlacklister(conf.Ignore["resource"]),
		Replacer:              filters.NewReplacer(conf.ReplaceTags),
		PrioritySampler:       sampler.NewPrioritySampler(conf, dynConf),
		ErrorsSampler:         sampler.NewErrorsSampler(conf),
		RareSampler:           sampler.NewRareSampler(conf),
		NoPrioritySampler:     sampler.NewNoPrioritySampler(conf),
		EventProcessor:        newEventProcessor(conf),
		StatsWriter:           writer.NewStatsWriter(conf, statsChan, telemetryCollector),
		obfuscator:            obfuscate.NewObfuscator(oconf),
		cardObfuscator:        newCreditCardsObfuscator(conf.Obfuscation.CreditCards),
		In:                    in,
		conf:                  conf,
		ctx:                   ctx,
		DebugServer:           api.NewDebugServer(conf),
	}
	agnt.Receiver = api.NewHTTPReceiver(conf, dynConf, in, agnt, telemetryCollector)
	agnt.OTLPReceiver = api.NewOTLPReceiver(in, conf)
	agnt.TraceWriter = writer.NewTraceWriter(conf, agnt.PrioritySampler, agnt.ErrorsSampler, agnt.RareSampler, telemetryCollector)
	return agnt
}

// Run starts routers routines and individual pieces then stop them when the exit order is received.
func (a *ServerlessAgent) Run() {
	timing.Start(metrics.Client)
	defer timing.Stop()
	for _, starter := range []interface{ Start() }{
		a.Receiver,
		a.Concentrator,
		a.ClientStatsAggregator,
		a.PrioritySampler,
		a.ErrorsSampler,
		a.NoPrioritySampler,
		a.EventProcessor,
		a.OTLPReceiver,
		a.DebugServer,
	} {
		starter.Start()
	}

	go a.TraceWriter.Run()
	go a.StatsWriter.Run()

	// Having GOMAXPROCS/2 processor threads is
	// enough to keep the downstream writer busy.
	// Having more processor threads would not speed
	// up processing, but just expand memory.
	workers := runtime.GOMAXPROCS(0) / 2
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		go a.work()
	}

	a.loop()
}

// FlushSync flushes traces sychronously. This method only works when the agent is configured in synchronous flushing
// mode via the apm_config.sync_flush option.
func (a *ServerlessAgent) FlushSync() {
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

func (a *ServerlessAgent) work() {
	for {
		p, ok := <-a.In
		if !ok {
			return
		}
		a.Process(p)
	}

}

func (a *ServerlessAgent) loop() {
	<-a.ctx.Done()
	log.Info("Exiting...")

	a.OTLPReceiver.Stop() // Stop OTLPReceiver before Receiver to avoid sending to closed channel
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
		a.obfuscator,
		a.cardObfuscator,
		a.DebugServer,
	} {
		stopper.Stop()
	}
}

// setRootSpanTags sets up any necessary tags on the root span.
func (a *ServerlessAgent) setRootSpanTags(root *pb.Span) {
	clientSampleRate := sampler.GetGlobalRate(root)
	sampler.SetClientRate(root, clientSampleRate)
}

// setFirstTraceTags sets additional tags on the first trace for each service processed by the agent,
// so that we can see that the service has successfully onboarded onto APM.
func (a *ServerlessAgent) setFirstTraceTags(root *pb.Span) {
	if a.conf == nil || a.conf.InstallSignature.InstallID == "" || root == nil {
		return
	}
	if _, alreadySeenService := a.firstSpanMap.LoadOrStore(root.Service, true); !alreadySeenService {
		// The install time and type can also be set on the trace by the tracer,
		// in which case we do not want the agent to overwrite them.
		if _, ok := traceutil.GetMeta(root, tagInstallID); !ok {
			traceutil.SetMeta(root, tagInstallID, a.conf.InstallSignature.InstallID)
		}
		if _, ok := traceutil.GetMeta(root, tagInstallType); !ok {
			traceutil.SetMeta(root, tagInstallType, a.conf.InstallSignature.InstallType)
		}
		if _, ok := traceutil.GetMeta(root, tagInstallTime); !ok {
			traceutil.SetMeta(root, tagInstallTime, strconv.FormatInt(a.conf.InstallSignature.InstallTime, 10))
		}
	}
}

// Process is the default work unit that receives a trace, transforms it and
// passes it downstream.
func (a *ServerlessAgent) Process(p *api.Payload) {
	if len(p.Chunks()) == 0 {
		log.Debugf("Skipping received empty payload")
		return
	}
	now := time.Now()
	defer timing.Since("datadog.trace_agent.internal.process_payload_ms", now)
	ts := p.Source
	sampledChunks := new(writer.SampledChunks)
	statsInput := stats.NewStatsInput(len(p.TracerPayload.Chunks), p.TracerPayload.ContainerID, p.ClientComputedStats, a.conf)

	p.TracerPayload.Env = traceutil.NormalizeTag(p.TracerPayload.Env)

	a.discardSpans(p)

	for i := 0; i < len(p.Chunks()); {
		chunk := p.Chunk(i)
		if len(chunk.Spans) == 0 {
			log.Debugf("Skipping received empty trace")
			p.RemoveChunk(i)
			continue
		}

		tracen := int64(len(chunk.Spans))
		ts.SpansReceived.Add(tracen)
		err := a.normalizeTrace(p.Source, chunk.Spans)
		if err != nil {
			log.Debugf("Dropping invalid trace: %s", err)
			ts.SpansDropped.Add(tracen)
			p.RemoveChunk(i)
			continue
		}

		// Root span is used to carry some trace-level metadata, such as sampling rate and priority.
		root := traceutil.GetRoot(chunk.Spans)
		setChunkAttributesFromRoot(chunk, root)
		if !a.Blacklister.Allows(root) {
			log.Debugf("Trace rejected by ignore resources rules. root: %v", root)
			ts.TracesFiltered.Inc()
			ts.SpansFiltered.Add(tracen)
			p.RemoveChunk(i)
			continue
		}

		if filteredByTags(root, a.conf.RequireTags, a.conf.RejectTags, a.conf.RequireTagsRegex, a.conf.RejectTagsRegex) {
			log.Debugf("Trace rejected as it fails to meet tag requirements. root: %v", root)
			ts.TracesFiltered.Inc()
			ts.SpansFiltered.Add(tracen)
			p.RemoveChunk(i)
			continue
		}

		// Extra sanitization steps of the trace.
		for _, span := range chunk.Spans {
			for k, v := range a.conf.GlobalTags {
				if k == tagOrigin {
					chunk.Origin = v
				} else {
					traceutil.SetMeta(span, k, v)
				}
			}
			if a.ModifySpan != nil {
				a.ModifySpan(chunk, span)
			}
			a.obfuscateSpan(span)
			a.Truncate(span)
			if p.ClientComputedTopLevel {
				traceutil.UpdateTracerTopLevel(span)
			}
		}
		a.Replacer.Replace(chunk.Spans)

		a.setRootSpanTags(root)
		if !p.ClientComputedTopLevel {
			// Figure out the top-level spans now as it involves modifying the Metrics map
			// which is not thread-safe while samplers and Concentrator might modify it too.
			traceutil.ComputeTopLevel(chunk.Spans)
		}

		a.setPayloadAttributes(p, root, chunk)

		pt := processedTrace(p, chunk, root)
		if !p.ClientComputedStats {
			statsInput.Traces = append(statsInput.Traces, *pt.Clone())
		}

		keep, numEvents := a.sample(now, ts, pt)
		if !keep && len(pt.TraceChunk.Spans) == 0 {
			// The entire trace was dropped and no spans were kept.
			p.RemoveChunk(i)
			continue
		}
		p.ReplaceChunk(i, pt.TraceChunk)

		if !pt.TraceChunk.DroppedTrace {
			// Now that we know this trace has been sampled,
			// if this is the first trace we have processed since restart,
			// set a special set of tags on its root span to track that this
			// customer has successfully onboarded onto APM.
			a.setFirstTraceTags(root)
			sampledChunks.SpanCount += int64(len(pt.TraceChunk.Spans))
		}
		sampledChunks.EventCount += int64(numEvents)
		sampledChunks.Size += pt.TraceChunk.Msgsize()
		i++

		if sampledChunks.Size > writer.MaxPayloadSize {
			// payload size is getting big; split and flush what we have so far
			sampledChunks.TracerPayload = p.TracerPayload.Cut(i)
			i = 0
			sampledChunks.TracerPayload.Chunks = newChunksArray(sampledChunks.TracerPayload.Chunks)
			a.TraceWriter.In <- sampledChunks
			sampledChunks = new(writer.SampledChunks)
		}
	}
	sampledChunks.TracerPayload = p.TracerPayload
	sampledChunks.TracerPayload.Chunks = newChunksArray(p.TracerPayload.Chunks)
	if sampledChunks.Size > 0 {
		a.TraceWriter.In <- sampledChunks
	}
	if len(statsInput.Traces) > 0 {
		a.Concentrator.In <- statsInput
	}
}

func (a *ServerlessAgent) setPayloadAttributes(p *api.Payload, root *pb.Span, chunk *pb.TraceChunk) {
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
}

// discardSpans removes all spans for which the provided DiscardFunction function returns true
func (a *ServerlessAgent) discardSpans(p *api.Payload) {
	if a.DiscardSpan == nil {
		return
	}
	for _, chunk := range p.Chunks() {
		n := 0
		for _, span := range chunk.Spans {
			if !a.DiscardSpan(span) {
				chunk.Spans[n] = span
				n++
			}
		}
		// set everything at the back of the array to nil to avoid memory leaking
		// since we're going to have garbage elements at the back of the slice.
		for i := n; i < len(chunk.Spans); i++ {
			chunk.Spans[i] = nil
		}
		chunk.Spans = chunk.Spans[:n]
	}
}

func (a *ServerlessAgent) processStats(in *pb.ClientStatsPayload, lang, tracerVersion string) *pb.ClientStatsPayload {
	enableContainers := a.conf.HasFeature("enable_cid_stats") || (a.conf.FargateOrchestrator != config.OrchestratorUnknown)
	if !enableContainers || a.conf.HasFeature("disable_cid_stats") {
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
			a.normalizeStatsGroup(b, lang)
			if !a.Blacklister.AllowsStat(b) {
				continue
			}
			a.obfuscateStatsGroup(b)
			a.Replacer.ReplaceStatsGroup(b)
			group.Stats[n] = b
			n++
		}
		in.Stats[i].Stats = group.Stats[:n]
		mergeDuplicates(in.Stats[i])
	}
	return in
}

// ProcessStats processes incoming client stats in from the given tracer.
func (a *ServerlessAgent) ProcessStats(in *pb.ClientStatsPayload, lang, tracerVersion string) {
	a.ClientStatsAggregator.In <- a.processStats(in, lang, tracerVersion)
}

// sample performs all sampling on the processedTrace modifying it as needed and returning if the trace should be kept and the number of events in the trace
func (a *ServerlessAgent) sample(now time.Time, ts *info.TagStats, pt *traceutil.ProcessedTrace) (keep bool, numEvents int) {
	// We have a `keep` that is different from pt's `DroppedTrace` field as `DroppedTrace` will be sent to intake.
	// For example: We want to maintain the overall trace level sampling decision for a trace with Analytics Events
	// where a trace might be marked as DroppedTrace true, but we still sent analytics events in that ProcessedTrace.
	keep, checkAnalyticsEvents := a.traceSampling(now, ts, pt)

	var events []*pb.Span
	if checkAnalyticsEvents {
		events = a.getAnalyzedEvents(pt, ts)
	}
	if !keep {
		modified := sampler.SingleSpanSampling(pt)
		if !modified {
			// If there were no sampled spans, and we're not keeping the trace, let's use the analytics events
			// This is OK because SSS is a replacement for analytics events so both should not be configured
			// And when analytics events are fully gone we can get rid of all this
			pt.TraceChunk.Spans = events
		} else if len(events) > 0 {
			log.Warnf("Detected both analytics events AND single span sampling in the same trace. Single span sampling wins because App Analytics is deprecated.")
		}
	}

	return keep, len(events)
}

// traceSampling reports whether the chunk should be kept as a trace, setting "DroppedTrace" on the chunk
func (a *ServerlessAgent) traceSampling(now time.Time, ts *info.TagStats, pt *traceutil.ProcessedTrace) (keep bool, checkAnalyticsEvents bool) {
	priority, hasPriority := sampler.GetSamplingPriority(pt.TraceChunk)

	if hasPriority {
		ts.TracesPerSamplingPriority.CountSamplingPriority(priority)
	} else {
		ts.TracesPriorityNone.Inc()
	}
	if a.conf.HasFeature("error_rare_sample_tracer_drop") {
		// We skip analytics events when a trace is marked as manual drop (aka priority -1)
		// Note that we DON'T skip single span sampling. We only do this for historical
		// reasons and analytics events are deprecated so hopefully this can all go away someday.
		if isManualUserDrop(priority, pt) {
			return false, false
		}
	} else { // This path to be deleted once manualUserDrop detection is available on all tracers for P < 1.
		if priority < 0 {
			return false, false
		}
	}
	sampled := a.runSamplers(now, *pt, hasPriority)
	pt.TraceChunk.DroppedTrace = !sampled

	return sampled, true
}

// getAnalyzedEvents returns any sampled analytics events in the ProcessedTrace
func (a *ServerlessAgent) getAnalyzedEvents(pt *traceutil.ProcessedTrace, ts *info.TagStats) []*pb.Span {
	numEvents, numExtracted, events := a.EventProcessor.Process(pt)
	ts.EventsExtracted.Add(numExtracted)
	ts.EventsSampled.Add(numEvents)
	return events
}

// runSamplers runs all the agent's samplers on pt and returns the sampling decision
// along with the sampling rate.
func (a *ServerlessAgent) runSamplers(now time.Time, pt traceutil.ProcessedTrace, hasPriority bool) bool {
	if hasPriority {
		return a.samplePriorityTrace(now, pt)
	}
	return a.sampleNoPriorityTrace(now, pt)
}

// samplePriorityTrace samples traces with priority set on them. PrioritySampler and
// ErrorSampler are run in parallel. The RareSampler catches traces with rare top-level
// or measured spans that are not caught by PrioritySampler and ErrorSampler.
func (a *ServerlessAgent) samplePriorityTrace(now time.Time, pt traceutil.ProcessedTrace) bool {
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
func (a *ServerlessAgent) sampleNoPriorityTrace(now time.Time, pt traceutil.ProcessedTrace) bool {
	if traceContainsError(pt.TraceChunk.Spans) {
		return a.ErrorsSampler.Sample(now, pt.TraceChunk.Spans, pt.Root, pt.TracerEnv)
	}
	return a.NoPrioritySampler.Sample(now, pt.TraceChunk.Spans, pt.Root, pt.TracerEnv)
}

// SetGlobalTagsUnsafe sets global tags to the agent configuration. Unsafe for concurrent use.
func (a *ServerlessAgent) SetGlobalTagsUnsafe(tags map[string]string) {
	a.conf.GlobalTags = tags
}

// normalize makes sure a Span is properly initialized and encloses the minimum required info, returning error if it
// is invalid beyond repair
func (a *ServerlessAgent) normalize(ts *info.TagStats, s *pb.Span) error {
	if s.TraceID == 0 {
		ts.TracesDropped.TraceIDZero.Inc()
		return fmt.Errorf("TraceID is zero (reason:trace_id_zero): %s", s)
	}
	if s.SpanID == 0 {
		ts.TracesDropped.SpanIDZero.Inc()
		return fmt.Errorf("SpanID is zero (reason:span_id_zero): %s", s)
	}
	svc, err := traceutil.NormalizeService(s.Service, ts.Lang)
	switch err {
	case traceutil.ErrEmpty:
		ts.SpansMalformed.ServiceEmpty.Inc()
		log.Debugf("Fixing malformed trace. Service is empty (reason:service_empty), setting span.service=%s: %s", s.Service, s)
	case traceutil.ErrTooLong:
		ts.SpansMalformed.ServiceTruncate.Inc()
		log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", traceutil.MaxServiceLen, s)
	case traceutil.ErrInvalid:
		ts.SpansMalformed.ServiceInvalid.Inc()
		log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s: %s", s.Service, svc, s)
	}
	s.Service = svc

	pSvc, ok := s.Meta[peerServiceKey]
	if ok {
		ps, err := traceutil.NormalizePeerService(pSvc)
		switch err {
		case traceutil.ErrTooLong:
			ts.SpansMalformed.PeerServiceTruncate.Inc()
			log.Debugf("Fixing malformed trace. peer.service is too long (reason:peer_service_truncate), truncating peer.service to length=%d: %s", traceutil.MaxServiceLen, ps)
		case traceutil.ErrInvalid:
			ts.SpansMalformed.PeerServiceInvalid.Inc()
			log.Debugf("Fixing malformed trace. peer.service is invalid (reason:peer_service_invalid), replacing invalid peer.service=%s with empty string", pSvc)
		default:
			if err != nil {
				log.Debugf("Unexpected error in peer.service normalization from original value (%s) to new value (%s): %s", pSvc, ps, err)
			}
		}
		s.Meta[peerServiceKey] = ps
	}

	if a.conf.HasFeature("component2name") {
		// This feature flag determines the component tag to become the span name.
		//
		// It works around the incompatibility between Opentracing and Datadog where the
		// Opentracing operation name is many times invalid as a Datadog operation name (e.g. "/")
		// and in Datadog terms it's the resource. Here, we aim to make the component the
		// operation name to provide a better product experience.
		if v, ok := s.Meta["component"]; ok {
			s.Name = v
		}
	}
	s.Name, err = traceutil.NormalizeName(s.Name)
	switch err {
	case traceutil.ErrEmpty:
		ts.SpansMalformed.SpanNameEmpty.Inc()
		log.Debugf("Fixing malformed trace. Name is empty (reason:span_name_empty), setting span.name=%s: %s", s.Name, s)
	case traceutil.ErrTooLong:
		ts.SpansMalformed.SpanNameTruncate.Inc()
		log.Debugf("Fixing malformed trace. Name is too long (reason:span_name_truncate), truncating span.name to length=%d: %s", traceutil.MaxServiceLen, s)
	case traceutil.ErrInvalid:
		ts.SpansMalformed.SpanNameInvalid.Inc()
		log.Debugf("Fixing malformed trace. Name is invalid (reason:span_name_invalid), setting span.name=%s: %s", s.Name, s)
	}

	if s.Resource == "" {
		ts.SpansMalformed.ResourceEmpty.Inc()
		log.Debugf("Fixing malformed trace. Resource is empty (reason:resource_empty), setting span.resource=%s: %s", s.Name, s)
		s.Resource = s.Name
	}

	// ParentID, TraceID and SpanID set in the client could be the same
	// Supporting the ParentID == TraceID == SpanID for the root span, is compliant
	// with the Zipkin implementation. Furthermore, as described in the PR
	// https://github.com/openzipkin/zipkin/pull/851 the constraint that the
	// root span's ``trace id = span id`` has been removed
	if s.ParentID == s.TraceID && s.ParentID == s.SpanID {
		s.ParentID = 0
		log.Debugf("span.normalize: `ParentID`, `TraceID` and `SpanID` are the same; `ParentID` set to 0: %d", s.TraceID)
	}

	// Start & Duration as nanoseconds timestamps
	// if s.Start is very little, less than year 2000 probably a unit issue so discard
	// (or it is "le bug de l'an 2000")
	if s.Duration < 0 {
		ts.SpansMalformed.InvalidDuration.Inc()
		log.Debugf("Fixing malformed trace. Duration is invalid (reason:invalid_duration), setting span.duration=0: %s", s)
		s.Duration = 0
	}
	if s.Duration > math.MaxInt64-s.Start {
		ts.SpansMalformed.InvalidDuration.Inc()
		log.Debugf("Fixing malformed trace. Duration is too large and causes overflow (reason:invalid_duration), setting span.duration=0: %s", s)
		s.Duration = 0
	}
	if s.Start < Year2000NanosecTS {
		ts.SpansMalformed.InvalidStartDate.Inc()
		log.Debugf("Fixing malformed trace. Start date is invalid (reason:invalid_start_date), setting span.start=time.now(): %s", s)
		now := time.Now().UnixNano()
		s.Start = now - s.Duration
		if s.Start < 0 {
			s.Start = now
		}
	}

	if len(s.Type) > MaxTypeLen {
		ts.SpansMalformed.TypeTruncate.Inc()
		log.Debugf("Fixing malformed trace. Type is too long (reason:type_truncate), truncating span.type to length=%d: %s", MaxTypeLen, s)
		s.Type = traceutil.TruncateUTF8(s.Type, MaxTypeLen)
	}
	if env, ok := s.Meta["env"]; ok {
		s.Meta["env"] = traceutil.NormalizeTag(env)
	}
	if sc, ok := s.Meta["http.status_code"]; ok {
		if !isValidStatusCode(sc) {
			ts.SpansMalformed.InvalidHTTPStatusCode.Inc()
			log.Debugf("Fixing malformed trace. HTTP status code is invalid (reason:invalid_http_status_code), dropping invalid http.status_code=%s: %s", sc, s)
			delete(s.Meta, "http.status_code")
		}
	}
	return nil
}

// normalizeTrace takes a trace and
// * rejects the trace if there is a trace ID discrepancy between 2 spans
// * rejects the trace if two spans have the same span_id
// * rejects empty traces
// * rejects traces where at least one span cannot be normalized
// * return the normalized trace and an error:
//   - nil if the trace can be accepted
//   - a reason tag explaining the reason the traces failed normalization
func (a *ServerlessAgent) normalizeTrace(ts *info.TagStats, t pb.Trace) error {
	if len(t) == 0 {
		ts.TracesDropped.EmptyTrace.Inc()
		return errors.New("trace is empty (reason:empty_trace)")
	}

	spanIDs := make(map[uint64]struct{})
	firstSpan := t[0]

	for _, span := range t {
		if span == nil {
			continue
		}
		if firstSpan == nil {
			firstSpan = span
		}
		if span.TraceID != firstSpan.TraceID {
			ts.TracesDropped.ForeignSpan.Inc()
			return fmt.Errorf("trace has foreign span (reason:foreign_span): %s", span)
		}
		if err := a.normalize(ts, span); err != nil {
			return err
		}
		if _, ok := spanIDs[span.SpanID]; ok {
			ts.SpansMalformed.DuplicateSpanID.Inc()
			log.Debugf("Found malformed trace with duplicate span ID (reason:duplicate_span_id): %s", span)
		}
		spanIDs[span.SpanID] = struct{}{}
	}

	return nil
}

func (a *ServerlessAgent) normalizeStatsGroup(b *pb.ClientGroupedStats, lang string) {
	b.Name, _ = traceutil.NormalizeName(b.Name)
	b.Service, _ = traceutil.NormalizeService(b.Service, lang)
	if b.Resource == "" {
		b.Resource = b.Name
	}
	b.Resource, _ = a.TruncateResource(b.Resource)
}

func (a *ServerlessAgent) obfuscateSpan(span *pb.Span) {
	o := a.obfuscator
	switch span.Type {
	case "sql", "cassandra":
		if span.Resource == "" {
			return
		}
		oq, err := o.ObfuscateSQLString(span.Resource)
		if err != nil {
			// we have an error, discard the SQL to avoid polluting user resources.
			log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
			span.Resource = textNonParsable
			traceutil.SetMeta(span, tagSQLQuery, textNonParsable)
			return
		}

		span.Resource = oq.Query
		if len(oq.Metadata.TablesCSV) > 0 {
			traceutil.SetMeta(span, "sql.tables", oq.Metadata.TablesCSV)
		}
		traceutil.SetMeta(span, tagSQLQuery, oq.Query)
	case "redis":
		span.Resource = o.QuantizeRedisString(span.Resource)
		if a.conf.Obfuscation.Redis.Enabled {
			if span.Meta == nil || span.Meta[tagRedisRawCommand] == "" {
				return
			}
			if a.conf.Obfuscation.Redis.RemoveAllArgs {
				span.Meta[tagRedisRawCommand] = o.RemoveAllRedisArgs(span.Meta[tagRedisRawCommand])
				return
			}
			span.Meta[tagRedisRawCommand] = o.ObfuscateRedisString(span.Meta[tagRedisRawCommand])
		}
	case "memcached":
		if !a.conf.Obfuscation.Memcached.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagMemcachedCommand] == "" {
			return
		}
		span.Meta[tagMemcachedCommand] = o.ObfuscateMemcachedString(span.Meta[tagMemcachedCommand])
	case "web", "http":
		if span.Meta == nil || span.Meta[tagHTTPURL] == "" {
			return
		}
		span.Meta[tagHTTPURL] = o.ObfuscateURLString(span.Meta[tagHTTPURL])
	case "mongodb":
		if !a.conf.Obfuscation.Mongo.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagMongoDBQuery] == "" {
			return
		}
		span.Meta[tagMongoDBQuery] = o.ObfuscateMongoDBString(span.Meta[tagMongoDBQuery])
	case "elasticsearch":
		if !a.conf.Obfuscation.ES.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagElasticBody] == "" {
			return
		}
		span.Meta[tagElasticBody] = o.ObfuscateElasticSearchString(span.Meta[tagElasticBody])
	}
}

func (a *ServerlessAgent) obfuscateStatsGroup(b *pb.ClientGroupedStats) {
	o := a.obfuscator
	switch b.Type {
	case "sql", "cassandra":
		oq, err := o.ObfuscateSQLString(b.Resource)
		if err != nil {
			log.Errorf("Error obfuscating stats group resource %q: %v", b.Resource, err)
			b.Resource = textNonParsable
		} else {
			b.Resource = oq.Query
		}
	case "redis":
		b.Resource = o.QuantizeRedisString(b.Resource)
	}
}

// Truncate checks that the span resource, meta and metrics are within the max length
// and modifies them if they are not
func (a *ServerlessAgent) Truncate(s *pb.Span) {
	r, ok := a.TruncateResource(s.Resource)
	if !ok {
		log.Debugf("span.truncate: truncated `Resource` (max %d chars): %s", a.conf.MaxResourceLen, s.Resource)
	}
	s.Resource = r

	// Error - Nothing to do
	// Optional data, Meta & Metrics can be nil
	// Soft fail on those
	for k, v := range s.Meta {
		modified := false

		if len(k) > MaxMetaKeyLen {
			log.Debugf("span.truncate: truncating `Meta` key (max %d chars): %s", MaxMetaKeyLen, k)
			delete(s.Meta, k)
			k = traceutil.TruncateUTF8(k, MaxMetaKeyLen) + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			v = traceutil.TruncateUTF8(v, MaxMetaValLen) + "..."
			modified = true
		}

		if modified {
			s.Meta[k] = v
		}
	}
	for k, v := range s.Metrics {
		if len(k) > MaxMetricsKeyLen {
			log.Debugf("span.truncate: truncating `Metrics` key (max %d chars): %s", MaxMetricsKeyLen, k)
			delete(s.Metrics, k)
			k = traceutil.TruncateUTF8(k, MaxMetricsKeyLen) + "..."

			s.Metrics[k] = v
		}
	}
}

// TruncateResource truncates a span's resource to the maximum allowed length.
// It returns true if the input was below the max size.
func (a *ServerlessAgent) TruncateResource(r string) (string, bool) {
	return traceutil.TruncateUTF8(r, a.conf.MaxResourceLen), len(r) <= a.conf.MaxResourceLen
}
