// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/api/loader"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// keyStatsComputed specifies the resource attribute key which indicates if stats have been
// computed for the resource spans.
const keyStatsComputed = "_dd.stats_computed"

var _ (ptraceotlp.GRPCServer) = (*OTLPReceiver)(nil)

// OTLPReceiver implements an OpenTelemetry Collector receiver which accepts incoming
// data on two ports for both plain HTTP and gRPC.
type OTLPReceiver struct {
	ptraceotlp.UnimplementedGRPCServer
	wg                 sync.WaitGroup      // waits for a graceful shutdown
	grpcsrv            *grpc.Server        // the running GRPC server on a started receiver, if enabled
	out                chan<- *Payload     // the outgoing payload channel
	conf               *config.AgentConfig // receiver config
	cidProvider        IDProvider          // container ID provider
	statsd             statsd.ClientInterface
	timing             timing.Reporter
	grpcMaxRecvMsgSize int
}

// NewOTLPReceiver returns a new OTLPReceiver which sends any incoming traces down the out channel.
func NewOTLPReceiver(out chan<- *Payload, cfg *config.AgentConfig, statsd statsd.ClientInterface, timing timing.Reporter) *OTLPReceiver {
	operationAndResourceNamesV2GateEnabled := !cfg.HasFeature("disable_operation_and_resource_name_logic_v2")
	operationAndResourceNamesV2GateEnabledVal := 0.0
	if operationAndResourceNamesV2GateEnabled {
		operationAndResourceNamesV2GateEnabledVal = 1.0
	}
	_ = statsd.Gauge("datadog.trace_agent.otlp.operation_and_resource_names_v2_gate_enabled", operationAndResourceNamesV2GateEnabledVal, nil, 1)

	spanNameAsResourceNameEnabledVal := 0.0
	if cfg.OTLPReceiver.SpanNameAsResourceName {
		log.Warnf("Detected SpanNameAsResourceName in config - this feature will be deprecated in a future version. Please remove it and set \"operation.name\" attribute on your spans instead. See the migration guide at https://docs.datadoghq.com/opentelemetry/guide/migrate/migrate_operation_names/")
		spanNameAsResourceNameEnabledVal = 1.0
	}
	_ = statsd.Gauge("datadog.trace_agent.otlp.span_name_as_resource_name_enabled", spanNameAsResourceNameEnabledVal, nil, 1)
	spanNameRemappingsEnabledVal := 0.0
	if len(cfg.OTLPReceiver.SpanNameRemappings) > 0 {
		log.Warnf("Detected SpanNameRemappings in config - this feature will be deprecated in a future version. Please remove it to access new operation name functionality. See the migration guide at https://docs.datadoghq.com/opentelemetry/guide/migrate/migrate_operation_names/")
		spanNameRemappingsEnabledVal = 1.0
	}
	_ = statsd.Gauge("datadog.trace_agent.otlp.span_name_remappings_enabled", spanNameRemappingsEnabledVal, nil, 1)
	computeTopLevelBySpanKindVal := 0.0
	if cfg.HasFeature("enable_otlp_compute_top_level_by_span_kind") {
		computeTopLevelBySpanKindVal = 1.0
	}
	_ = statsd.Gauge("datadog.trace_agent.otlp.compute_top_level_by_span_kind", computeTopLevelBySpanKindVal, nil, 1)
	enableReceiveResourceSpansV2Val := 1.0
	if cfg.HasFeature("disable_receive_resource_spans_v2") {
		enableReceiveResourceSpansV2Val = 0.0
	}
	_ = statsd.Gauge("datadog.trace_agent.otlp.enable_receive_resource_spans_v2", enableReceiveResourceSpansV2Val, nil, 1)
	grpcMaxRecvMsgSize := 10 * 1024 * 1024
	if cfg.OTLPReceiver.GrpcMaxRecvMsgSizeMib > 0 {
		grpcMaxRecvMsgSize = cfg.OTLPReceiver.GrpcMaxRecvMsgSizeMib * 1024 * 1024
	}
	return &OTLPReceiver{out: out, conf: cfg, cidProvider: NewIDProvider(cfg.ContainerProcRoot, cfg.ContainerIDFromOriginInfo), statsd: statsd, timing: timing, grpcMaxRecvMsgSize: grpcMaxRecvMsgSize}
}

// Start starts the OTLPReceiver, if any of the servers were configured as active.
func (o *OTLPReceiver) Start() {
	cfg := o.conf.OTLPReceiver
	if cfg.GRPCPort != 0 {
		var ln net.Listener
		var err error
		// When using the trace-loader, the OTLP listener might be provided as an already opened file descriptor
		// so we try to get a listener from it, and fallback to listening on the given address if it fails
		if grpcFDStr, ok := os.LookupEnv("DD_OTLP_CONFIG_GRPC_FD"); ok {
			ln, err = loader.GetListenerFromFD(grpcFDStr, "otlp_conn")
			if err == nil {
				log.Debugf("Using OTLP listener from file descriptor %s", grpcFDStr)
			} else {
				log.Errorf("Error creating OTLP listener from file descriptor %s: %v", grpcFDStr, err)
			}
		}
		if ln == nil {
			// if the fd was not provided, or we failed to get a listener from it, listen on the given address
			ln, err = loader.GetTCPListener(fmt.Sprintf("%s:%d", cfg.BindHost, cfg.GRPCPort))
		}

		if err != nil {
			log.Criticalf("Error starting OpenTelemetry gRPC server: %v", err)
		} else {
			opts := []grpc.ServerOption{
				grpc.MaxRecvMsgSize(o.grpcMaxRecvMsgSize),
				grpc.MaxConcurrentStreams(1), // Each payload must be sent to processing stage before we decode the next.
			}

			// OTLP trace ingestion doesn't need generic gRPC metrics interceptors
			// since we collect business-specific metrics (spans, traces, payloads) via StatsD

			o.grpcsrv = grpc.NewServer(opts...)
			ptraceotlp.RegisterGRPCServer(o.grpcsrv, o)
			o.wg.Add(1)
			go func() {
				defer o.wg.Done()
				if err := o.grpcsrv.Serve(ln); err != nil {
					log.Criticalf("Error starting OpenTelemetry gRPC server: %v", err)
				}
			}()
			log.Debugf("Listening to core Agent for OTLP traces on internal gRPC port (http://%s:%d, internal use only). Check core Agent logs for information on the OTLP ingest status.", cfg.BindHost, cfg.GRPCPort)
		}
	}
}

// Stop stops any running server.
func (o *OTLPReceiver) Stop() {
	if o.grpcsrv != nil {
		go o.grpcsrv.Stop()
	}
	o.wg.Wait()
}

// Export implements ptraceotlp.Server
func (o *OTLPReceiver) Export(ctx context.Context, in ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	defer o.timing.Since("datadog.trace_agent.otlp.process_grpc_request_ms", time.Now())
	md, _ := metadata.FromIncomingContext(ctx)
	_ = o.statsd.Count("datadog.trace_agent.otlp.payload", 1, tagsFromHeaders(http.Header(md)), 1)
	o.processRequest(ctx, http.Header(md), in)
	return ptraceotlp.NewExportResponse(), nil
}

func tagsFromHeaders(h http.Header) []string {
	tags := []string{"endpoint_version:opentelemetry_grpc_v1"}
	if v := fastHeaderGet(h, header.Lang); v != "" {
		tags = append(tags, "lang:"+v)
	}
	if v := fastHeaderGet(h, header.LangVersion); v != "" {
		tags = append(tags, "lang_version:"+v)
	}
	if v := fastHeaderGet(h, header.LangInterpreter); v != "" {
		tags = append(tags, "interpreter:"+v)
	}
	if v := fastHeaderGet(h, header.LangInterpreterVendor); v != "" {
		tags = append(tags, "lang_vendor:"+v)
	}
	return tags
}

// fastHeaderGet returns the given key from the header, avoiding the caonical transformation of key
// that is normally applied by http.Header.Get.
func fastHeaderGet(h http.Header, canonicalKey string) string {
	if h == nil {
		return ""
	}
	v, ok := h[canonicalKey]
	if !ok || len(v) == 0 {
		return ""
	}
	return v[0]
}

// processRequest processes the incoming request in.
func (o *OTLPReceiver) processRequest(ctx context.Context, header http.Header, in ptraceotlp.ExportRequest) {
	for i := 0; i < in.Traces().ResourceSpans().Len(); i++ {
		rspans := in.Traces().ResourceSpans().At(i)
		o.ReceiveResourceSpans(ctx, rspans, header, nil)
	}
}

// knuthFactor represents a large, prime number ideal for Knuth's Multiplicative Hashing.
// Warning: do not change this number. It is shared with other probabilistic samplers
// in the agent, the Datadog libraries, and in OpenTelemetry. This ensures consistency
// in a distributed system.
const knuthFactor = uint64(1111111111111111111)

// samplingRate returns the rate as defined by the probabilistic sampler.
func (o *OTLPReceiver) samplingRate() float64 {
	rate := o.conf.OTLPReceiver.ProbabilisticSampling / 100
	if rate <= 0 || rate >= 1 {
		// assume that the user wants to keep the trace since he has sent it from
		// his SDK and introduced no sampling mechanisms anywhere else.
		return 1
	}
	return rate
}

// sample returns the sampling priority to apply to a trace with the trace ID tid.
func (o *OTLPReceiver) sample(tid uint64) sampler.SamplingPriority {
	rate := o.samplingRate()
	if rate == 1 {
		return sampler.PriorityAutoKeep
	}
	// the trace ID (tid) is hashed using Knuth's multiplicative hash
	hash := tid * knuthFactor
	if hash < uint64(rate*math.MaxUint64) {
		// if the hash result falls into the rate percentage of the entire distribution
		// of possibly trace IDs (uint64), we sample it.
		return sampler.PriorityAutoKeep
	}
	return sampler.PriorityAutoDrop
}

// SetOTelAttributeTranslator sets the attribute translator to be used by this OTLPReceiver
func (o *OTLPReceiver) SetOTelAttributeTranslator(attrstrans *attributes.Translator) {
	o.conf.OTLPReceiver.AttributesTranslator = attrstrans
}

// ReceiveResourceSpans processes the given rspans and returns the source that it identified from processing them.
func (o *OTLPReceiver) ReceiveResourceSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler attributes.HostFromAttributesHandler) source.Source {
	if o.conf.HasFeature("disable_receive_resource_spans_v2") {
		return o.receiveResourceSpansV1(ctx, rspans, httpHeader, hostFromAttributesHandler)
	}
	return o.receiveResourceSpansV2(ctx, rspans, isHeaderTrue(header.ComputedStats, httpHeader.Get(header.ComputedStats)), hostFromAttributesHandler)
}

func (o *OTLPReceiver) receiveResourceSpansV2(ctx context.Context, rspans ptrace.ResourceSpans, clientComputedStats bool, hostFromAttributesHandler attributes.HostFromAttributesHandler) source.Source {
	otelres := rspans.Resource()
	resourceAttributes := otelres.Attributes()

	tracesByID := make(map[uint64]pb.Trace)
	priorityByID := make(map[uint64]sampler.SamplingPriority)
	var spancount int64
	for i := 0; i < rspans.ScopeSpans().Len(); i++ {
		libspans := rspans.ScopeSpans().At(i)
		for j := 0; j < libspans.Spans().Len(); j++ {
			otelspan := libspans.Spans().At(j)
			spancount++
			traceID := traceutil.OTelTraceIDToUint64(otelspan.TraceID())
			if tracesByID[traceID] == nil {
				tracesByID[traceID] = pb.Trace{}
			}
			ddspan := transform.OtelSpanToDDSpan(otelspan, otelres, libspans.Scope(), o.conf)

			if p, ok := ddspan.Metrics["_sampling_priority_v1"]; ok {
				priorityByID[traceID] = sampler.SamplingPriority(p)
			}
			tracesByID[traceID] = append(tracesByID[traceID], ddspan)
		}
	}

	lang := traceutil.GetOTelAttrVal(resourceAttributes, true, string(semconv.TelemetrySDKLanguageKey))
	tagstats := &info.TagStats{
		Tags: info.Tags{
			Lang:            lang,
			TracerVersion:   fmt.Sprintf("otlp-%s", traceutil.GetOTelAttrVal(resourceAttributes, true, string(semconv.TelemetrySDKVersionKey))),
			EndpointVersion: "opentelemetry_grpc_v1",
		},
		Stats: info.NewStats(),
	}

	tags := tagstats.AsTags()
	_ = o.statsd.Count("datadog.trace_agent.otlp.spans", spancount, tags, 1)
	_ = o.statsd.Count("datadog.trace_agent.otlp.traces", int64(len(tracesByID)), tags, 1)
	p := Payload{
		Source:                 tagstats,
		ClientComputedStats:    traceutil.GetOTelAttrVal(resourceAttributes, true, keyStatsComputed) != "" || clientComputedStats,
		ClientComputedTopLevel: o.conf.HasFeature("enable_otlp_compute_top_level_by_span_kind"),
	}
	// Get the hostname or set to empty if source is empty
	var hostname string
	src, srcok := o.conf.OTLPReceiver.AttributesTranslator.ResourceToSource(ctx, rspans.Resource(), traceutil.SignalTypeSet, hostFromAttributesHandler)
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			hostname = src.Identifier
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			hostname = ""
		}
	} else {
		// fallback hostname
		hostname = o.conf.Hostname
		src = source.Source{Kind: source.HostnameKind, Identifier: hostname}
	}
	if o.conf.OTLPReceiver.IgnoreMissingDatadogFields {
		hostname = ""
	}
	if incomingHostname := traceutil.GetOTelAttrVal(resourceAttributes, true, transform.KeyDatadogHost); incomingHostname != "" {
		hostname = incomingHostname
	}

	containerID := traceutil.GetOTelAttrVal(resourceAttributes, true, transform.KeyDatadogContainerID)
	if containerID == "" && !o.conf.OTLPReceiver.IgnoreMissingDatadogFields {
		containerID = traceutil.GetOTelAttrVal(resourceAttributes, true, string(semconv.ContainerIDKey), string(semconv.K8SPodUIDKey))
	}

	env := traceutil.GetOTelAttrVal(resourceAttributes, true, transform.KeyDatadogEnvironment)
	if env == "" && !o.conf.OTLPReceiver.IgnoreMissingDatadogFields {
		env = traceutil.GetOTelAttrVal(resourceAttributes, true, string(semconv127.DeploymentEnvironmentNameKey), string(semconv.DeploymentEnvironmentKey))
	}
	p.TracerPayload = &pb.TracerPayload{
		Hostname:      hostname,
		Chunks:        o.createChunks(tracesByID, priorityByID),
		Env:           env,
		ContainerID:   containerID,
		LanguageName:  tagstats.Lang,
		TracerVersion: tagstats.TracerVersion,
	}

	var flattenedTags string
	if incomingContainerTags := traceutil.GetOTelAttrVal(resourceAttributes, true, transform.KeyDatadogContainerTags); incomingContainerTags != "" {
		flattenedTags = incomingContainerTags
	} else if !o.conf.OTLPReceiver.IgnoreMissingDatadogFields {
		ctags := attributes.ContainerTagsFromResourceAttributes(resourceAttributes)
		payloadTags := flatten(ctags)

		// Populate container tags by calling ContainerTags tagger from configuration
		if tags := getContainerTags(o.conf.ContainerTags, containerID); tags != "" {
			appendTags(payloadTags, tags)
		} else {
			// we couldn't obtain any container tags
			if src.Kind == source.AWSECSFargateKind {
				// but we have some information from the source provider that we can add
				appendTags(payloadTags, src.Tag())
			}
		}
		flattenedTags = payloadTags.String()
	}

	if len(flattenedTags) > 0 {
		p.TracerPayload.Tags = map[string]string{
			tagContainersTags: flattenedTags,
		}
	}

	o.out <- &p
	return src
}

func (o *OTLPReceiver) receiveResourceSpansV1(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler attributes.HostFromAttributesHandler) source.Source {
	// each rspans is coming from a different resource and should be considered
	// a separate payload; typically there is only one item in this slice
	src, srcok := o.conf.OTLPReceiver.AttributesTranslator.ResourceToSource(ctx, rspans.Resource(), traceutil.SignalTypeSet, hostFromAttributesHandler)
	hostFromMap := func(m map[string]string, key string) {
		// hostFromMap sets the hostname to m[key] if it is set.
		if v, ok := m[key]; ok {
			src = source.Source{Kind: source.HostnameKind, Identifier: v}
			srcok = true
		}
	}

	attr := rspans.Resource().Attributes()
	rattr := make(map[string]string, attr.Len())
	attr.Range(func(k string, v pcommon.Value) bool {
		rattr[k] = v.AsString()
		return true
	})
	if !srcok {
		hostFromMap(rattr, "_dd.hostname")
	}
	_, env := transform.GetFirstFromMap(rattr, string(semconv127.DeploymentEnvironmentNameKey), string(semconv.DeploymentEnvironmentKey))
	lang := rattr[string(semconv.TelemetrySDKLanguageKey)]
	if lang == "" {
		lang = fastHeaderGet(httpHeader, header.Lang)
	}
	_, containerID := transform.GetFirstFromMap(rattr, string(semconv.ContainerIDKey), string(semconv.K8SPodUIDKey))
	if containerID == "" {
		containerID = o.cidProvider.GetContainerID(ctx, httpHeader)
	}
	tagstats := &info.TagStats{
		Tags: info.Tags{
			Lang:            lang,
			LangVersion:     fastHeaderGet(httpHeader, header.LangVersion),
			Interpreter:     fastHeaderGet(httpHeader, header.LangInterpreter),
			LangVendor:      fastHeaderGet(httpHeader, header.LangInterpreterVendor),
			TracerVersion:   fmt.Sprintf("otlp-%s", rattr[string(semconv.TelemetrySDKVersionKey)]),
			EndpointVersion: "opentelemetry_grpc_v1",
		},
		Stats: info.NewStats(),
	}
	tracesByID := make(map[uint64]pb.Trace)
	priorityByID := make(map[uint64]sampler.SamplingPriority)
	var spancount int64
	for i := 0; i < rspans.ScopeSpans().Len(); i++ {
		libspans := rspans.ScopeSpans().At(i)
		lib := libspans.Scope()
		for j := 0; j < libspans.Spans().Len(); j++ {
			spancount++
			span := libspans.Spans().At(j)
			traceID := traceutil.OTelTraceIDToUint64(span.TraceID())
			if tracesByID[traceID] == nil {
				tracesByID[traceID] = pb.Trace{}
			}
			ddspan := o.convertSpan(rspans.Resource(), lib, span)
			if !srcok {
				// if we didn't find a hostname at the resource level
				// try and see if the span has a hostname set
				hostFromMap(ddspan.Meta, "_dd.hostname")
			}
			if env == "" {
				// no env at resource level, try the first span
				if v := ddspan.Meta["env"]; v != "" {
					env = v
				}
			}
			if containerID == "" {
				// no cid at resource level, grab what we can
				_, containerID = transform.GetFirstFromMap(ddspan.Meta, string(semconv.ContainerIDKey), string(semconv.K8SPodUIDKey))
			}
			if p, ok := ddspan.Metrics["_sampling_priority_v1"]; ok {
				priorityByID[traceID] = sampler.SamplingPriority(p)
			}
			tracesByID[traceID] = append(tracesByID[traceID], ddspan)
		}
	}
	tags := tagstats.AsTags()
	_ = o.statsd.Count("datadog.trace_agent.otlp.spans", spancount, tags, 1)
	_ = o.statsd.Count("datadog.trace_agent.otlp.traces", int64(len(tracesByID)), tags, 1)
	p := Payload{
		Source:                 tagstats,
		ClientComputedStats:    rattr[keyStatsComputed] != "" || isHeaderTrue(header.ComputedStats, httpHeader.Get(header.ComputedStats)),
		ClientComputedTopLevel: o.conf.HasFeature("enable_otlp_compute_top_level_by_span_kind") || isHeaderTrue(header.ComputedTopLevel, httpHeader.Get(header.ComputedTopLevel)),
	}
	if env == "" {
		env = o.conf.DefaultEnv
	}

	// Get the hostname or set to empty if source is empty
	var hostname string
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			hostname = src.Identifier
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			hostname = ""
		}
	} else {
		// fallback hostname
		hostname = o.conf.Hostname
		src = source.Source{Kind: source.HostnameKind, Identifier: hostname}
	}
	p.TracerPayload = &pb.TracerPayload{
		Hostname:        hostname,
		Chunks:          o.createChunks(tracesByID, priorityByID),
		Env:             normalize.NormalizeTagValue(env),
		ContainerID:     containerID,
		LanguageName:    tagstats.Lang,
		LanguageVersion: tagstats.LangVersion,
		TracerVersion:   tagstats.TracerVersion,
	}
	ctags := attributes.ContainerTagsFromResourceAttributes(attr)
	payloadTags := flatten(ctags)
	if tags := getContainerTags(o.conf.ContainerTags, containerID); tags != "" {
		appendTags(payloadTags, tags)
	} else {
		// we couldn't obtain any container tags
		if src.Kind == source.AWSECSFargateKind {
			// but we have some information from the source provider that we can add
			appendTags(payloadTags, src.Tag())
		}
	}
	if payloadTags.Len() > 0 {
		p.TracerPayload.Tags = map[string]string{
			tagContainersTags: payloadTags.String(),
		}
	}

	o.out <- &p
	return src
}

func appendTags(str *strings.Builder, tags string) {
	if str.Len() > 0 {
		str.WriteByte(',')
	}
	str.WriteString(tags)
}

func flatten(m map[string]string) *strings.Builder {
	var str strings.Builder
	for k, v := range m {
		if str.Len() > 0 {
			str.WriteByte(',')
		}
		str.WriteString(k)
		str.WriteString(":")
		str.WriteString(v)
	}
	return &str
}

// createChunks creates a set of pb.TraceChunk's based on two maps:
// - a map from trace ID to the spans sharing that trace ID
// - a map of user-set sampling priorities by trace ID, if set
func (o *OTLPReceiver) createChunks(tracesByID map[uint64]pb.Trace, prioritiesByID map[uint64]sampler.SamplingPriority) []*pb.TraceChunk {
	traceChunks := make([]*pb.TraceChunk, 0, len(tracesByID))
	for k, spans := range tracesByID {
		if len(spans) == 0 {
			continue
		}
		rate := strconv.FormatFloat(o.samplingRate(), 'f', 2, 64)
		chunk := &pb.TraceChunk{
			Tags:  map[string]string{"_dd.otlp_sr": rate},
			Spans: spans,
		}
		if o.conf.ProbabilisticSamplerEnabled {
			// SamplingPriority is not related to the decision of ProbabilisticSampler and ErrorsSampler.
			chunk.Priority = int32(sampler.PriorityNone)
			// Skip making a sampling decision at this point.
			// Either ProbabilisticSampler enabled by this config or ErrorsSampler will decide.
			traceChunks = append(traceChunks, chunk)
			continue
		}
		var samplingPriorty sampler.SamplingPriority
		var decisionMaker string
		if p, ok := prioritiesByID[k]; ok {
			// a manual decision has been made by the user
			samplingPriorty = p
			decisionMaker = "-4"
		} else {
			// we use the probabilistic sampler to decide
			samplingPriorty = o.sample(k)
			decisionMaker = "-9"
		}
		// `_dd.p.dm` must not be set even if a drop decision is applied to the trace here.
		// Traces with a drop decision by the OTLPReceiver's probabilistic sampler are re-evaluated by ErrorsSampler later.
		if samplingPriorty.IsKeep() {
			traceutil.SetMeta(spans[0], "_dd.p.dm", decisionMaker)
		}
		chunk.Priority = int32(samplingPriorty)
		traceChunks = append(traceChunks, chunk)
	}
	return traceChunks
}

// convertSpan converts the span in to a Datadog span, and uses the res resource and the lib instrumentation
// library attributes to further augment it.
func (o *OTLPReceiver) convertSpan(res pcommon.Resource, lib pcommon.InstrumentationScope, in ptrace.Span) *pb.Span {
	traceID := [16]byte(in.TraceID())
	span := &pb.Span{
		TraceID:  traceutil.OTelTraceIDToUint64(traceID),
		SpanID:   traceutil.OTelSpanIDToUint64(in.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(in.ParentSpanID()),
		Start:    int64(in.StartTimestamp()),
		Duration: int64(in.EndTimestamp()) - int64(in.StartTimestamp()),
		Meta:     make(map[string]string, res.Attributes().Len()),
		Metrics:  map[string]float64{},
	}
	res.Attributes().Range(func(k string, v pcommon.Value) bool {
		transform.SetMetaOTLP(span, k, v.AsString())
		return true
	})

	spanKind := in.Kind()
	if o.conf.HasFeature("enable_otlp_compute_top_level_by_span_kind") {
		computeTopLevelAndMeasured(span, spanKind)
	}

	transform.SetMetaOTLP(span, "otel.trace_id", hex.EncodeToString(traceID[:]))
	transform.SetMetaOTLP(span, "span.kind", traceutil.OTelSpanKindName(spanKind))
	if _, ok := span.Meta["version"]; !ok {
		if ver, ok := res.Attributes().Get(string(semconv.ServiceVersionKey)); ok && ver.AsString() != "" {
			transform.SetMetaOTLP(span, "version", ver.AsString())
		}
	}
	if in.Events().Len() > 0 {
		transform.SetMetaOTLP(span, "events", transform.MarshalEvents(in.Events()))
	}
	transform.TagSpanIfContainsExceptionEvent(in, span)
	if in.Links().Len() > 0 {
		transform.SetMetaOTLP(span, "_dd.span_links", transform.MarshalLinks(in.Links()))
	}

	var gotMethodFromNewConv bool
	var gotStatusCodeFromNewConv bool

	in.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			transform.SetMetricOTLP(span, k, v.Double())
		case pcommon.ValueTypeInt:
			transform.SetMetricOTLP(span, k, float64(v.Int()))
		default:
			// Exclude Datadog APM conventions.
			// These are handled below explicitly.
			if k != "http.method" && k != "http.status_code" {
				transform.SetMetaOTLP(span, k, v.AsString())
			}
		}

		// `http.method` was renamed to `http.request.method` in the HTTP stabilization from v1.23.
		// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
		// `http.method` is also the Datadog APM convention for the HTTP method.
		// We check both conventions and use the new one if it is present.
		// See https://datadoghq.atlassian.net/wiki/spaces/APM/pages/2357395856/Span+attributes#[inlineExtension]HTTP
		if k == "http.request.method" {
			gotMethodFromNewConv = true
			transform.SetMetaOTLP(span, "http.method", v.AsString())
		} else if k == "http.method" && !gotMethodFromNewConv {
			transform.SetMetaOTLP(span, "http.method", v.AsString())
		}

		// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
		// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
		// `http.status_code` is also the Datadog APM convention for the HTTP status code.
		// We check both conventions and use the new one if it is present.
		// See https://datadoghq.atlassian.net/wiki/spaces/APM/pages/2357395856/Span+attributes#[inlineExtension]HTTP
		if k == "http.response.status_code" {
			gotStatusCodeFromNewConv = true
			transform.SetMetaOTLP(span, "http.status_code", v.AsString())
		} else if k == "http.status_code" && !gotStatusCodeFromNewConv {
			transform.SetMetaOTLP(span, "http.status_code", v.AsString())
		}

		return true
	})
	if _, ok := span.Meta["env"]; !ok {
		if _, env := transform.GetFirstFromMap(span.Meta, string(semconv127.DeploymentEnvironmentNameKey), string(semconv.DeploymentEnvironmentKey)); env != "" {
			transform.SetMetaOTLP(span, "env", normalize.NormalizeTag(env))
		}
	}

	// Check for db.namespace and conditionally set db.name
	if _, ok := span.Meta["db.name"]; !ok {
		if dbNamespace := traceutil.GetOTelAttrValInResAndSpanAttrs(in, res, false, string(semconv127.DBNamespaceKey)); dbNamespace != "" {
			transform.SetMetaOTLP(span, "db.name", dbNamespace)
		}
	}

	if in.TraceState().AsRaw() != "" {
		transform.SetMetaOTLP(span, "w3c.tracestate", in.TraceState().AsRaw())
	}
	if lib.Name() != "" {
		transform.SetMetaOTLP(span, string(semconv117.OtelLibraryNameKey), lib.Name())
	}
	if lib.Version() != "" {
		transform.SetMetaOTLP(span, string(semconv117.OtelLibraryVersionKey), lib.Version())
	}
	transform.SetMetaOTLP(span, string(semconv117.OtelStatusCodeKey), in.Status().Code().String())
	if msg := in.Status().Message(); msg != "" {
		transform.SetMetaOTLP(span, string(semconv117.OtelStatusDescriptionKey), msg)
	}
	span.Error = transform.Status2Error(in.Status(), in.Events(), span.Meta)
	if transform.OperationAndResourceNameV2Enabled(o.conf) {
		span.Name = traceutil.GetOTelOperationNameV2(in, res)
	} else {
		if span.Name == "" {
			name := in.Name()
			if !o.conf.OTLPReceiver.SpanNameAsResourceName {
				name = traceutil.OTelSpanKindName(in.Kind())
				if lib.Name() != "" {
					name = lib.Name() + "." + name
				} else {
					name = "opentelemetry." + name
				}
			}
			if v, ok := o.conf.OTLPReceiver.SpanNameRemappings[name]; ok {
				name = v
			}
			span.Name = name
		}
	}
	if span.Service == "" {
		span.Service = "OTLPResourceNoServiceName"
	}
	if span.Resource == "" {
		if transform.OperationAndResourceNameV2Enabled(o.conf) {
			span.Resource = traceutil.GetOTelResourceV2(in, res)
		} else {
			if r := resourceFromTags(span.Meta); r != "" {
				span.Resource = r
			} else {
				span.Resource = in.Name()
			}
		}
	}
	if span.Type == "" {
		span.Type = traceutil.SpanKind2Type(in, res)
	}
	return span
}

// resourceFromTags attempts to deduce a more accurate span resource from the given list of tags meta.
// If this is not possible, it returns an empty string.
func resourceFromTags(meta map[string]string) string {
	// `http.method` was renamed to `http.request.method` in the HTTP stabilization from v1.23.
	// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
	if _, m := transform.GetFirstFromMap(meta, "http.request.method", "http.method"); m != "" {
		// use the HTTP method + route (if available)
		if _, route := transform.GetFirstFromMap(meta, string(semconv.HTTPRouteKey), "grpc.path"); route != "" {
			return m + " " + route
		}
		return m
	} else if m := meta[string(semconv.MessagingOperationKey)]; m != "" {
		// use the messaging operation
		if _, dest := transform.GetFirstFromMap(meta, string(semconv.MessagingDestinationKey), string(semconv117.MessagingDestinationNameKey)); dest != "" {
			return m + " " + dest
		}
		return m
	} else if m := meta[string(semconv.RPCMethodKey)]; m != "" {
		// use the RPC method
		if svc := meta[string(semconv.RPCServiceKey)]; svc != "" {
			// ...and service if available
			return m + " " + svc
		}
		return m
	} else if typ := meta[string(semconv117.GraphqlOperationTypeKey)]; typ != "" {
		// Enrich GraphQL query resource names.
		// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
		if name := meta[string(semconv117.GraphqlOperationNameKey)]; name != "" {
			return typ + " " + name
		}
		return typ
	}
	return ""
}

// computeTopLevelAndMeasured updates the span's top-level and measured attributes.
//
// An OTLP span is considered top-level if it is a root span or has a span kind of server or consumer.
// An OTLP span is marked as measured if it has a span kind of client or producer.
func computeTopLevelAndMeasured(span *pb.Span, spanKind ptrace.SpanKind) {
	if span.ParentID == 0 {
		// span is a root span
		traceutil.SetTopLevel(span, true)
	}
	if spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer {
		// span is a server-side span
		traceutil.SetTopLevel(span, true)
	}
	if spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer {
		// span is a client-side span, not top-level but we still want stats
		traceutil.SetMeasured(span, true)
	}
}
