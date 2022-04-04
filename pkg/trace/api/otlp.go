// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	semconv "go.opentelemetry.io/collector/model/semconv/v1.6.1"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// otlpProtocolHTTP specifies that the incoming connection was made over plain HTTP.
	otlpProtocolHTTP = "http"
	// otlpProtocolGRPC specifies that the incoming connection was made over gRPC.
	otlpProtocolGRPC = "grpc"
)

// OTLPReceiver implements an OpenTelemetry Collector receiver which accepts incoming
// data on two ports for both plain HTTP and gRPC.
type OTLPReceiver struct {
	wg      sync.WaitGroup      // waits for a graceful shutdown
	httpsrv *http.Server        // the running HTTP server on a started receiver, if enabled
	grpcsrv *grpc.Server        // the running GRPC server on a started receiver, if enabled
	out     chan<- *Payload     // the outgoing payload channel
	conf    *config.AgentConfig // receiver config
}

// NewOTLPReceiver returns a new OTLPReceiver which sends any incoming traces down the out channel.
func NewOTLPReceiver(out chan<- *Payload, cfg *config.AgentConfig) *OTLPReceiver {
	return &OTLPReceiver{out: out, conf: cfg}
}

// Start starts the OTLPReceiver, if any of the servers were configured as active.
func (o *OTLPReceiver) Start() {
	cfg := o.conf.OTLPReceiver
	if cfg.HTTPPort != 0 {
		o.httpsrv = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.BindHost, cfg.HTTPPort),
			Handler: o,
		}
		o.wg.Add(1)
		go func() {
			defer o.wg.Done()
			if err := o.httpsrv.ListenAndServe(); err != nil {
				if err != http.ErrServerClosed {
					log.Criticalf("Error starting OpenTelemetry HTTP server: %v", err)
				}
			}
		}()
		log.Debugf("Listening to core Agent for OTLP traces on internal HTTP port (http://%s:%d, internal use only). Check core Agent logs for information on the OTLP ingest status.", cfg.BindHost, cfg.HTTPPort)
	}
	if cfg.GRPCPort != 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.BindHost, cfg.GRPCPort))
		if err != nil {
			log.Criticalf("Error starting OpenTelemetry gRPC server: %v", err)
		} else {
			o.grpcsrv = grpc.NewServer()
			ptraceotlp.RegisterServer(o.grpcsrv, o)
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
	if o.httpsrv != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		go func() {
			if err := o.httpsrv.Shutdown(timeout); err != nil {
				log.Errorf("Error shutting down OTLP HTTP server: %v", err)
			}
			cancel()
		}()
	}
	if o.grpcsrv != nil {
		go o.grpcsrv.Stop()
	}
	o.wg.Wait()
}

// Export implements ptraceotlp.Server
func (o *OTLPReceiver) Export(ctx context.Context, in ptraceotlp.Request) (ptraceotlp.Response, error) {
	defer timing.Since("datadog.trace_agent.otlp.process_grpc_request_ms", time.Now())
	md, _ := metadata.FromIncomingContext(ctx)
	metrics.Count("datadog.trace_agent.otlp.payload", 1, tagsFromHeaders(http.Header(md), otlpProtocolGRPC), 1)
	o.processRequest(otlpProtocolGRPC, http.Header(md), in)
	return ptraceotlp.NewResponse(), nil
}

// ServeHTTP implements http.Handler
func (o *OTLPReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer timing.Since("datadog.trace_agent.otlp.process_http_request_ms", time.Now())
	mtags := tagsFromHeaders(req.Header, otlpProtocolHTTP)
	metrics.Count("datadog.trace_agent.otlp.payload", 1, mtags, 1)

	r := req.Body
	if req.Header.Get("Content-Encoding") == "gzip" {
		gzipr, err := gzip.NewReader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			metrics.Count("datadog.trace_agent.otlp.error", 1, append(mtags, "reason:corrupt_gzip"), 1)
			return
		}
		r = gzipr
	}
	rd := apiutil.NewLimitedReader(r, o.conf.OTLPReceiver.MaxRequestBytes)
	slurp, err := ioutil.ReadAll(rd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		metrics.Count("datadog.trace_agent.otlp.error", 1, append(mtags, "reason:read_body"), 1)
		return
	}
	metrics.Count("datadog.trace_agent.otlp.bytes", int64(len(slurp)), mtags, 1)
	in := ptraceotlp.NewRequest()
	switch getMediaType(req) {
	case "application/x-protobuf":
		if err := in.UnmarshalProto(slurp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			metrics.Count("datadog.trace_agent.otlp.error", 1, append(mtags, "reason:decode_proto"), 1)
			return
		}
	case "application/json":
		fallthrough
	default:
		if err := in.UnmarshalJSON(slurp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			metrics.Count("datadog.trace_agent.otlp.error", 1, append(mtags, "reason:decode_json"), 1)
			return
		}
	}
	o.processRequest(otlpProtocolHTTP, req.Header, in)
}

func tagsFromHeaders(h http.Header, protocol string) []string {
	tags := []string{"endpoint_version:opentelemetry_" + protocol + "_v1"}
	if v := fastHeaderGet(h, headerLang); v != "" {
		tags = append(tags, "lang:"+v)
	}
	if v := fastHeaderGet(h, headerLangVersion); v != "" {
		tags = append(tags, "lang_version:"+v)
	}
	if v := fastHeaderGet(h, headerLangInterpreter); v != "" {
		tags = append(tags, "interpreter:"+v)
	}
	if v := fastHeaderGet(h, headerLangInterpreterVendor); v != "" {
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

// processRequest processes the incoming request in. It marks it as received by the given protocol
// using the given headers.
func (o *OTLPReceiver) processRequest(protocol string, header http.Header, in ptraceotlp.Request) {
	for i := 0; i < in.Traces().ResourceSpans().Len(); i++ {
		rspans := in.Traces().ResourceSpans().At(i)
		o.ReceiveResourceSpans(rspans, header, protocol)
	}
}

// OTLPIngestSummary returns a summary of the received resource spans.
type OTLPIngestSummary struct {
	// Hostname indicates the hostname of the passed resource spans.
	Hostname string
	// Tags returns a set of Datadog-specific tags which are relevant for identifying
	// the source of the passed resource spans.
	Tags []string
}

// ReceiveResourceSpans processes the given rspans and sends them to writer.
func (o *OTLPReceiver) ReceiveResourceSpans(rspans ptrace.ResourceSpans, header http.Header, protocol string) OTLPIngestSummary {
	// each rspans is coming from a different resource and should be considered
	// a separate payload; typically there is only one item in this slice
	attr := rspans.Resource().Attributes()
	rattr := make(map[string]string, attr.Len())
	attr.Range(func(k string, v pcommon.Value) bool {
		rattr[k] = v.AsString()
		return true
	})
	hostname, _ := attributes.HostnameFromAttributes(attr)
	if hostname == "" {
		hostname = rattr["_dd.hostname"]
	}
	env := rattr[string(semconv.AttributeDeploymentEnvironment)]
	lang := rattr[string(semconv.AttributeTelemetrySDKLanguage)]
	if lang == "" {
		lang = fastHeaderGet(header, headerLang)
	}
	containerID := rattr[string(semconv.AttributeContainerID)]
	if containerID == "" {
		containerID = rattr[string(semconv.AttributeK8SPodUID)]
	}
	if containerID == "" {
		containerID = fastHeaderGet(header, headerContainerID)
	}
	tagstats := &info.TagStats{
		Tags: info.Tags{
			Lang:            lang,
			LangVersion:     fastHeaderGet(header, headerLangVersion),
			Interpreter:     fastHeaderGet(header, headerLangInterpreter),
			LangVendor:      fastHeaderGet(header, headerLangInterpreterVendor),
			TracerVersion:   fmt.Sprintf("otlp-%s", rattr[string(semconv.AttributeTelemetrySDKVersion)]),
			EndpointVersion: fmt.Sprintf("opentelemetry_%s_v1", protocol),
		},
		Stats: info.NewStats(),
	}
	tracesByID := make(map[uint64]pb.Trace)
	for i := 0; i < rspans.ScopeSpans().Len(); i++ {
		libspans := rspans.ScopeSpans().At(i)
		lib := libspans.Scope()
		for i := 0; i < libspans.Spans().Len(); i++ {
			span := libspans.Spans().At(i)
			traceID := traceIDToUint64(span.TraceID().Bytes())
			if tracesByID[traceID] == nil {
				tracesByID[traceID] = pb.Trace{}
			}
			ddspan := o.convertSpan(rattr, lib, span)
			if hostname == "" {
				// if we didn't find a hostname at the resource level
				// try and see if the span has a hostname set
				if v := ddspan.Meta["_dd.hostname"]; v != "" {
					hostname = v
				}
			}
			if env == "" {
				// no env at resource level, try the first span
				if v := ddspan.Meta["env"]; v != "" {
					env = v
				}
			}
			if containerID == "" {
				// no cid at resource level, grab what we can
				if v := ddspan.Meta[string(semconv.AttributeK8SPodUID)]; v != "" {
					containerID = v
				}
				if v := ddspan.Meta[string(semconv.AttributeContainerID)]; v != "" {
					containerID = v
				}
			}
			tracesByID[traceID] = append(tracesByID[traceID], ddspan)
		}
	}
	tags := tagstats.AsTags()
	metrics.Count("datadog.trace_agent.otlp.spans", int64(rspans.ScopeSpans().Len()), tags, 1)
	metrics.Count("datadog.trace_agent.otlp.traces", int64(len(tracesByID)), tags, 1)
	traceChunks := make([]*pb.TraceChunk, 0, len(tracesByID))
	p := Payload{
		Source: tagstats,
	}
	for _, spans := range tracesByID {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			// auto-keep all incoming traces; it was already chosen as a keeper on
			// the client side.
			Priority: int32(sampler.PriorityAutoKeep),
			Spans:    spans,
		})
	}
	if env == "" {
		env = o.conf.DefaultEnv
	}
	if hostname == "" {
		hostname = o.conf.Hostname
	}
	p.TracerPayload = &pb.TracerPayload{
		Hostname:        hostname,
		Chunks:          traceChunks,
		Env:             traceutil.NormalizeTag(env),
		ContainerID:     containerID,
		LanguageName:    tagstats.Lang,
		LanguageVersion: tagstats.LangVersion,
		TracerVersion:   tagstats.TracerVersion,
	}
	if ctags := getContainerTags(o.conf.ContainerTags, containerID); ctags != "" {
		p.TracerPayload.Tags = map[string]string{
			tagContainersTags: ctags,
		}
	}
	select {
	case o.out <- &p:
		// 👍
	default:
		log.Warn("Payload in channel full. Dropped 1 payload.")
	}
	return OTLPIngestSummary{
		Hostname: hostname,
		Tags:     attributes.RunningTagsFromAttributes(attr),
	}
}

// marshalEvents marshals events into JSON.
func marshalEvents(events ptrace.SpanEventSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		var wrote bool
		str.WriteString("{")
		if v := e.Timestamp(); v != 0 {
			str.WriteString(`"time_unix_nano":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
			wrote = true
		}
		if v := e.Name(); v != "" {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"name":"`)
			str.WriteString(v)
			str.WriteString(`"`)
			wrote = true
		}
		if e.Attributes().Len() > 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"attributes":{`)
			j := 0
			e.Attributes().Range(func(k string, v pcommon.Value) bool {
				if j > 0 {
					str.WriteString(",")
				}
				str.WriteString(`"`)
				str.WriteString(k)
				str.WriteString(`":"`)
				str.WriteString(v.AsString())
				str.WriteString(`"`)
				j++
				return true
			})
			str.WriteString("}")
			wrote = true
		}
		if v := e.DroppedAttributesCount(); v != 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// convertSpan converts the span in to a Datadog span, and uses the rattr resource tags and the lib instrumentation
// library attributes to further augment it.
func (o *OTLPReceiver) convertSpan(rattr map[string]string, lib pcommon.InstrumentationScope, in ptrace.Span) *pb.Span {
	traceID := in.TraceID().Bytes()
	meta := make(map[string]string, len(rattr))
	for k, v := range rattr {
		meta[k] = v
	}
	span := &pb.Span{
		TraceID:  traceIDToUint64(traceID),
		SpanID:   spanIDToUint64(in.SpanID().Bytes()),
		ParentID: spanIDToUint64(in.ParentSpanID().Bytes()),
		Start:    int64(in.StartTimestamp()),
		Duration: int64(in.EndTimestamp()) - int64(in.StartTimestamp()),
		Meta:     meta,
		Metrics:  map[string]float64{},
	}
	span.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if _, ok := span.Meta["version"]; !ok {
		if ver := rattr[string(semconv.AttributeServiceVersion)]; ver != "" {
			span.Meta["version"] = ver
		}
	}
	if in.Events().Len() > 0 {
		span.Meta["events"] = marshalEvents(in.Events())
	}
	in.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			span.Metrics[k] = v.DoubleVal()
		case pcommon.ValueTypeInt:
			span.Metrics[k] = float64(v.IntVal())
		default:
			switch k {
			case "operation.name":
				span.Name = v.AsString()
			case "service.name":
				span.Service = v.AsString()
			case "resource.name":
				span.Resource = v.AsString()
			case "span.type":
				span.Type = v.AsString()
			case "analytics.event":
				if v, err := strconv.ParseBool(v.AsString()); err == nil {
					if v {
						span.Metrics[sampler.KeySamplingRateEventExtraction] = 1
					} else {
						span.Metrics[sampler.KeySamplingRateEventExtraction] = 0
					}
				}
			default:
				span.Meta[k] = v.AsString()
			}
		}
		return true
	})
	if ctags := attributes.ContainerTagFromAttributes(span.Meta); ctags != "" {
		span.Meta[tagContainersTags] = ctags
	}
	if _, ok := span.Meta["env"]; !ok {
		if env := span.Meta[string(semconv.AttributeDeploymentEnvironment)]; env != "" {
			span.Meta["env"] = traceutil.NormalizeTag(env)
		}
	}
	if in.TraceState() != ptrace.TraceStateEmpty {
		span.Meta["w3c.tracestate"] = string(in.TraceState())
	}
	if lib.Name() != "" {
		span.Meta[semconv.OtelLibraryName] = lib.Name()
	}
	if lib.Version() != "" {
		span.Meta[semconv.OtelLibraryVersion] = lib.Version()
	}
	span.Meta[semconv.OtelStatusCode] = in.Status().Code().String()
	if msg := in.Status().Message(); msg != "" {
		span.Meta[semconv.OtelStatusDescription] = msg
	}
	status2Error(in.Status(), in.Events(), span)
	if span.Name == "" {
		name := in.Name()
		if !o.conf.OTLPReceiver.SpanNameAsResourceName {
			name = spanKindName(in.Kind())
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
	if span.Service == "" {
		if svc := span.Meta[string(semconv.AttributePeerService)]; svc != "" {
			span.Service = svc
		} else if svc := rattr[string(semconv.AttributeServiceName)]; svc != "" {
			span.Service = svc
		} else {
			span.Service = "OTLPResourceNoServiceName"
		}
	}
	if span.Resource == "" {
		if r := resourceFromTags(span.Meta); r != "" {
			span.Resource = r
		} else {
			span.Resource = in.Name()
		}
	}
	if span.Type == "" {
		span.Type = spanKind2Type(in.Kind(), span)
	}
	return span
}

// resourceFromTags attempts to deduce a more accurate span resource from the given list of tags meta.
// If this is not possible, it returns an empty string.
func resourceFromTags(meta map[string]string) string {
	if m := meta[string(semconv.AttributeHTTPMethod)]; m != "" {
		// use the HTTP method + route (if available)
		if route := meta[string(semconv.AttributeHTTPRoute)]; route != "" {
			return m + " " + route
		} else if route := meta["grpc.path"]; route != "" {
			return m + " " + route
		}
		return m
	} else if m := meta[string(semconv.AttributeMessagingOperation)]; m != "" {
		// use the messaging operation
		if dest := meta[string(semconv.AttributeMessagingDestination)]; dest != "" {
			return m + " " + dest
		}
		return m
	} else if m := meta[string(semconv.AttributeRPCMethod)]; m != "" {
		// use the RPC method
		if svc := meta[string(semconv.AttributeRPCService)]; svc != "" {
			// ...and service if availabl
			return m + " " + svc
		}
		return m
	}
	return ""
}

// status2Error checks the given status and events and applies any potential error and messages
// to the given span attributes.
func status2Error(status ptrace.SpanStatus, events ptrace.SpanEventSlice, span *pb.Span) {
	if status.Code() != ptrace.StatusCodeError {
		return
	}
	span.Error = 1
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if strings.ToLower(e.Name()) != "exception" {
			continue
		}
		attrs := e.Attributes()
		if v, ok := attrs.Get(semconv.AttributeExceptionMessage); ok {
			span.Meta["error.msg"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionType); ok {
			span.Meta["error.type"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionStacktrace); ok {
			span.Meta["error.stack"] = v.AsString()
		}
	}
	if _, ok := span.Meta["error.msg"]; !ok {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			span.Meta["error.msg"] = status.Message()
		} else if httpcode, ok := span.Meta["http.status_code"]; ok {
			// we have status code that we can use as details
			if httptext, ok := span.Meta["http.status_text"]; ok {
				span.Meta["error.msg"] = fmt.Sprintf("%s %s", httpcode, httptext)
			} else {
				span.Meta["error.msg"] = httpcode
			}
		}
	}
}

// spanKind2Type returns a span's type based on the given kind and other present properties.
func spanKind2Type(kind ptrace.SpanKind, span *pb.Span) string {
	var typ string
	switch kind {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		typ = "http"
		db, ok := span.Meta[string(semconv.AttributeDBSystem)]
		if !ok {
			break
		}
		switch db {
		case "redis", "memcached":
			typ = "cache"
		default:
			typ = "db"
		}
	default:
		typ = "custom"
	}
	return typ
}

func traceIDToUint64(b [16]byte) uint64 {
	return binary.BigEndian.Uint64(b[len(b)-8:])
}

func spanIDToUint64(b [8]byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

var spanKindNames = map[int32]string{
	0: "unspecified",
	1: "internal",
	2: "server",
	3: "client",
	4: "producer",
	5: "consumer",
}

// spanKindName converts the given SpanKind to a valid Datadog span name.
func spanKindName(k ptrace.SpanKind) string {
	name, ok := spanKindNames[int32(k)]
	if !ok {
		return "unknown"
	}
	return name
}
