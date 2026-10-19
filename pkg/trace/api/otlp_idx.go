// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/otlpidx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// otlpIdxEnabled reports whether OTLP gRPC requests are decoded directly into the V1
// representation and processed by ProcessV1. Configurations the decoder doesn't implement
// (see otlpidx.Supported) and the disable_otlp_idx kill switch keep the pdata / v04 path.
func otlpIdxEnabled(conf *config.AgentConfig) bool {
	return otlpidx.Supported(conf) && !conf.HasFeature("disable_otlp_idx")
}

// rawExportRequest is a serialized ExportTraceServiceRequest, decoded by the Export handler
// rather than by the gRPC codec.
type rawExportRequest struct {
	buf mem.Buffer
}

// exportResponse is an empty ExportTraceServiceResponse.
type exportResponse struct{}

// otlpCodec is the gRPC codec of the OTLP receiver's server when the direct decoder is enabled.
// It hands the request bytes to the Export handler without copying them, and delegates any
// other message to the proto codec.
type otlpCodec struct {
	delegate encoding.CodecV2
}

func newOTLPCodec() otlpCodec {
	return otlpCodec{delegate: encoding.GetCodecV2("proto")}
}

func (c otlpCodec) Marshal(v any) (mem.BufferSlice, error) {
	if _, ok := v.(*exportResponse); ok {
		// An empty message has an empty encoding.
		return nil, nil
	}
	return c.delegate.Marshal(v)
}

func (c otlpCodec) Unmarshal(data mem.BufferSlice, v any) error {
	if r, ok := v.(*rawExportRequest); ok {
		// gRPC frees data once Unmarshal returns, so keep a reference until the handler is done.
		r.buf = data.MaterializeToBuffer(mem.DefaultBufferPool())
		return nil
	}
	return c.delegate.Unmarshal(data, v)
}

func (otlpCodec) Name() string { return "proto" }

// otlpRawTraceServer is implemented by OTLPReceiver for otlpTraceServiceDesc.
type otlpRawTraceServer interface {
	exportRaw(context.Context, *rawExportRequest) (*exportResponse, error)
}

// otlpTraceServiceDesc registers opentelemetry.proto.collector.trace.v1.TraceService with a
// handler that receives the serialized request, like ptraceotlp.RegisterGRPCServer but without
// decoding it into pdata.
var otlpTraceServiceDesc = grpc.ServiceDesc{
	ServiceName: "opentelemetry.proto.collector.trace.v1.TraceService",
	HandlerType: (*otlpRawTraceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Export",
			Handler:    otlpExportHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "opentelemetry/proto/collector/trace/v1/trace_service.proto",
}

//nolint:revive // context-as-argument: the signature is grpc.MethodHandler.
func otlpExportHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := &rawExportRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if in.buf != nil {
		defer in.buf.Free()
	}
	if interceptor == nil {
		return srv.(otlpRawTraceServer).exportRaw(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/opentelemetry.proto.collector.trace.v1.TraceService/Export",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(otlpRawTraceServer).exportRaw(ctx, req.(*rawExportRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// exportRaw is the gRPC Export handler when the direct decoder is enabled. The request is decoded
// into V1 payloads for ProcessV1; when the decoder rejects it, it falls back to pdata and the v04
// path, so that a decoder gap never drops data.
func (o *OTLPReceiver) exportRaw(ctx context.Context, in *rawExportRequest) (*exportResponse, error) {
	defer o.timing.Since("datadog.trace_agent.otlp.process_grpc_request_ms", time.Now())
	md, _ := metadata.FromIncomingContext(ctx)
	hdr := http.Header(md)
	_ = o.statsd.Count("datadog.trace_agent.otlp.payload", 1, tagsFromHeaders(hdr), 1)
	var b []byte
	if in.buf != nil {
		b = in.buf.ReadOnlyData()
	}
	payloads, err := otlpidx.Decode(ctx, b, otlpidx.Options{Conf: o.conf})
	if err == nil {
		o.receiveIdx(payloads)
		return &exportResponse{}, nil
	}
	log.Debugf("OTLP direct decoder rejected a request, falling back to pdata: %v", err)
	_ = o.statsd.Count("datadog.trace_agent.otlp.idx_fallback", 1, nil, 1)
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalProto(b); err != nil {
		// What gRPC returns when the pdata codec fails.
		return nil, status.Errorf(codes.Internal, "grpc: failed to unmarshal the received message: %v", err)
	}
	o.processRequest(ctx, hdr, req)
	return &exportResponse{}, nil
}

// receiveIdx sends decoded payloads to ProcessV1, mirroring processRequest and
// receiveResourceSpansV2 for the request metrics, source tags and client-computed flags.
func (o *OTLPReceiver) receiveIdx(payloads []otlpidx.Payload) {
	OTLPIngestAgentTracesRequests.Inc()
	var spans int
	for _, p := range payloads {
		spans += p.SpanCount
	}
	OTLPIngestAgentTracesEvents.Add(float64(spans))

	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.isStopped.Load() {
		log.Warn(errOTLPReceiverStopped)
		return
	}
	computedTopLevel := o.conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	for _, p := range payloads {
		tp := p.TracerPayload
		tagstats := &info.TagStats{
			Tags: info.Tags{
				Lang:            tp.LanguageName(),
				TracerVersion:   tp.TracerVersion(),
				EndpointVersion: "opentelemetry_grpc_v1",
			},
			Stats: info.NewStats(),
		}
		tags := tagstats.AsTags()
		_ = o.statsd.Count("datadog.trace_agent.otlp.spans", int64(p.SpanCount), tags, 1)
		_ = o.statsd.Count("datadog.trace_agent.otlp.traces", int64(len(tp.Chunks)), tags, 1)
		// Only the _dd.stats_computed resource attribute applies: the Datadog-Client-Computed-Stats
		// header is set by Datadog tracers and the DDOT exporter, never on OTLP gRPC requests.
		o.outV1 <- &PayloadV1{
			Source:                 tagstats,
			TracerPayload:          tp,
			ClientComputedStats:    p.StatsComputed != nil && *p.StatsComputed,
			ClientComputedTopLevel: computedTopLevel,
		}
	}
}
