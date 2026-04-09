// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// FuzzOTLPReceiveResourceSpansV2 fuzzes the OTLP trace ingestion path by feeding
// random protobuf-encoded ExportRequest payloads through the receiver. The OTLP
// path is a growing ingestion surface with complex nested structures (resources,
// scopes, spans, attributes, events, links) and zero prior fuzz coverage.
func FuzzOTLPReceiveResourceSpansV2(f *testing.F) {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		f.Fatalf("failed to create attributes translator: %v", err)
	}
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator

	out := make(chan *Payload, 100)
	defer close(out)
	// Drain the output channel to prevent blocking.
	go func() {
		for range out {
		}
	}()

	rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})

	// Seed corpus: valid OTLP requests of varying complexity.
	seeds := []ptraceotlp.ExportRequest{
		// Minimal: single span
		testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "testlib",
				LibVersion: "1.0",
				Attributes: map[string]interface{}{"service.name": "svc"},
				Spans:      []*testutil.OTLPSpan{{Name: "op"}},
			},
		}),
		// Multiple resource spans with events and links
		testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "libA",
				LibVersion: "2.0",
				Attributes: map[string]interface{}{"service.name": "web", "env": "prod"},
				Spans: []*testutil.OTLPSpan{
					{
						Name:       "/api/v1",
						Kind:       ptrace.SpanKindServer,
						TraceState: "dd=s:1",
						Attributes: map[string]interface{}{"http.status_code": 200, "http.method": "GET"},
						Events: []testutil.OTLPSpanEvent{
							{Name: "exception", Attributes: map[string]interface{}{"exception.message": "boom"}},
						},
						Links: []testutil.OTLPSpanLink{
							{TraceID: "fedcba98765432100123456789abcdef", SpanID: "abcdef0123456789"},
						},
						StatusCode: ptrace.StatusCodeError,
						StatusMsg:  "internal error",
					},
					{Name: "/health", Kind: ptrace.SpanKindInternal},
				},
			},
			{
				LibName:    "libB",
				LibVersion: "1.0",
				Attributes: map[string]interface{}{"service.name": "db"},
				Spans:      []*testutil.OTLPSpan{{Name: "SELECT", Kind: ptrace.SpanKindClient}},
			},
		}),
		// Empty request
		testutil.NewOTLPTracesRequest(nil),
	}

	for _, seed := range seeds {
		bts, err := seed.MarshalProto()
		if err != nil {
			f.Fatalf("failed to marshal seed: %v", err)
		}
		f.Add(bts)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		req := ptraceotlp.NewExportRequest()
		if err := req.UnmarshalProto(data); err != nil {
			return // invalid protobuf, skip
		}

		rspans := req.Traces().ResourceSpans()
		for i := 0; i < rspans.Len(); i++ {
			// ReceiveResourceSpans must not panic regardless of input.
			_, _ = rcv.ReceiveResourceSpans(
				context.Background(),
				rspans.At(i),
				http.Header{},
				nil,
			)
		}
	})
}

// FuzzOTLPReceiveResourceSpansV1 fuzzes the legacy V1 OTLP processing path.
func FuzzOTLPReceiveResourceSpansV1(f *testing.F) {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		f.Fatalf("failed to create attributes translator: %v", err)
	}
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	// Force V1 path
	cfg.Features["disable_receive_resource_spans_v2"] = struct{}{}

	out := make(chan *Payload, 100)
	defer close(out)
	go func() {
		for range out {
		}
	}()

	rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})

	// Reuse the same seed as V2
	seed := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "testlib",
			LibVersion: "1.0",
			Attributes: map[string]interface{}{"service.name": "svc"},
			Spans:      []*testutil.OTLPSpan{{Name: "op", Kind: ptrace.SpanKindServer}},
		},
	})
	bts, err := seed.MarshalProto()
	if err != nil {
		f.Fatalf("failed to marshal seed: %v", err)
	}
	f.Add(bts)

	f.Fuzz(func(t *testing.T, data []byte) {
		req := ptraceotlp.NewExportRequest()
		if err := req.UnmarshalProto(data); err != nil {
			return
		}

		rspans := req.Traces().ResourceSpans()
		for i := 0; i < rspans.Len(); i++ {
			_, _ = rcv.ReceiveResourceSpans(
				context.Background(),
				rspans.At(i),
				http.Header{},
				nil,
			)
		}
	})
}
