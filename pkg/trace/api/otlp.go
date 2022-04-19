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

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"

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

// Export implements ptraceotlp.TracesServer
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
		err := in.UnmarshalProto(slurp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			metrics.Count("datadog.trace_agent.otlp.error", 1, append(mtags, "reason:decode_proto"), 1)
			return
		}
	case "application/json":
		fallthrough
	default:
		err := in.UnmarshalJSON(slurp)
		if err != nil {
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
		// each rspans is coming from a different resource and should be considered
		// a separate payload; typically there is only one item in this slice
		attr := rspans.Resource().Attributes()
		rattr := make(map[string]string, attr.Len())
		attr.Range(func(k string, v pcommon.Value) bool {
			rattr[k] = v.AsString()
			return true
		})
		lang := rattr[string(semconv.AttributeTelemetrySDKLanguage)]
		if lang == "" {
			lang = fastHeaderGet(header, headerLang)
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
				tracesByID[traceID] = append(tracesByID[traceID], convertSpan(rattr, lib, span))
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
		p.TracerPayload = &pb.TracerPayload{
			Chunks:          traceChunks,
			ContainerID:     fastHeaderGet(header, headerContainerID),
			LanguageName:    tagstats.Lang,
			LanguageVersion: tagstats.LangVersion,
			TracerVersion:   tagstats.TracerVersion,
		}
		if ctags := getContainerTags(o.conf.ContainerTags, p.TracerPayload.ContainerID); ctags != "" {
			p.TracerPayload.Tags = map[string]string{
				tagContainersTags: ctags,
			}
		}
		o.out <- &p
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
func convertSpan(rattr map[string]string, lib pcommon.InstrumentationScope, in ptrace.Span) *pb.Span {
	name := spanKindName(in.Kind())
	if lib.Name() != "" {
		name = lib.Name() + "." + name
	} else {
		name = "opentelemetry." + name
	}
	traceID := in.TraceID().Bytes()
	meta := make(map[string]string, len(rattr))
	for k, v := range rattr {
		meta[k] = v
	}
	span := &pb.Span{
		Name:     name,
		TraceID:  traceIDToUint64(traceID),
		SpanID:   spanIDToUint64(in.SpanID().Bytes()),
		ParentID: spanIDToUint64(in.ParentSpanID().Bytes()),
		Start:    int64(in.StartTimestamp()),
		Duration: int64(in.EndTimestamp()) - int64(in.StartTimestamp()),
		Service:  rattr[string(semconv.AttributeServiceName)],
		Resource: in.Name(),
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
			span.Meta[k] = v.AsString()
		}
		return true
	})
	if _, ok := span.Meta["env"]; !ok {
		if env := span.Meta[string(semconv.AttributeDeploymentEnvironment)]; env != "" {
			span.Meta["env"] = env
		}
	}
	if in.TraceState() != ptrace.TraceStateEmpty {
		span.Meta["trace_state"] = string(in.TraceState())
	}
	if lib.Name() != "" {
		span.Meta["instrumentation_library.name"] = lib.Name()
	}
	if lib.Version() != "" {
		span.Meta["instrumentation_library.version"] = lib.Version()
	}
	if svc := span.Meta[string(semconv.AttributePeerService)]; svc != "" {
		span.Service = svc
	}
	if r := resourceFromTags(span.Meta); r != "" {
		span.Resource = r
	}
	span.Type = spanKind2Type(in.Kind(), span)
	status2Error(in.Status(), in.Events(), span)
	return span
}

// resourceFromTags attempts to deduce a more accurate span resource from the given list of tags meta.
// If this is not possible, it returns an empty string.
func resourceFromTags(meta map[string]string) string {
	var r string
	if m := meta[string(semconv.AttributeHTTPMethod)]; m != "" {
		r = m
		if route := meta[string(semconv.AttributeHTTPRoute)]; route != "" {
			r += " " + route
		} else if route := meta["grpc.path"]; route != "" {
			r += " " + route
		}
	} else if m := meta[string(semconv.AttributeMessagingOperation)]; m != "" {
		r = m
		if dest := meta[string(semconv.AttributeMessagingDestination)]; dest != "" {
			r += " " + dest
		}
	}
	return r
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
		e.Attributes().Range(func(k string, v pcommon.Value) bool {
			switch k {
			case string(semconv.AttributeExceptionMessage):
				span.Meta["error.msg"] = v.AsString()
			case string(semconv.AttributeExceptionType):
				span.Meta["error.type"] = v.AsString()
			case string(semconv.AttributeExceptionStacktrace):
				span.Meta["error.stack"] = v.AsString()
			}
			return true
		})
	}
	if _, ok := span.Meta["error.msg"]; !ok {
		if status.Message() != "" {
			span.Meta["error.msg"] = status.Message()
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
