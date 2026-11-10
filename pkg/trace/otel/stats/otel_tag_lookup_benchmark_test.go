// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

const (
	tagLookupSpans         = 500
	tagLookupResourceAttrs = 43
	tagLookupSpanAttrs     = 10
)

// tagLookupPeerAttrs are default peer tag precursors; each span carries 2 or 3 of them.
var tagLookupPeerAttrs = []string{"peer.service", "db.system", "db.name"}

// buildTagLookupTraces builds single-span traces under one resource with
// tagLookupResourceAttrs dummy attributes (none of them peer or primary tags).
// Each span has tagLookupSpanAttrs dummy attributes, 2-3 peer tag precursors
// and, on every other span, the given primary tag keys.
func buildTagLookupTraces(primaryTagKeys []string) ptrace.Traces {
	start := pcommon.NewTimestampFromTime(time.Now().Add(-time.Second))
	end := pcommon.NewTimestampFromTime(time.Now())
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rattrs := rs.Resource().Attributes()
	rattrs.PutStr("service.name", "svc")
	for i := 0; i < tagLookupResourceAttrs; i++ {
		rattrs.PutStr(fmt.Sprintf("resource.attr.%02d", i), "value")
	}
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	for s := 0; s < tagLookupSpans; s++ {
		span := spans.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: byte(s), 14: byte(s >> 8)})
		span.SetSpanID(pcommon.SpanID{7: byte(s), 6: byte(s >> 8)})
		span.SetKind(ptrace.SpanKindClient)
		span.SetName("client-op")
		span.SetStartTimestamp(start)
		span.SetEndTimestamp(end)
		attrs := span.Attributes()
		for i := 0; i < tagLookupSpanAttrs; i++ {
			attrs.PutStr(fmt.Sprintf("span.attr.%02d", i), "value")
		}
		for _, k := range tagLookupPeerAttrs[:2+s%2] {
			attrs.PutStr(k, "peer-value")
		}
		if s%2 == 0 {
			for _, k := range primaryTagKeys {
				attrs.PutStr(k, "primary-value")
			}
		}
	}
	return traces
}

// BenchmarkOTelStatsTagLookup measures the peer and primary tag lookup cost of
// OTLPTracesToConcentratorInputs.
func BenchmarkOTelStatsTagLookup(b *testing.B) {
	primary := []string{"customer.tier", "region.cell"}
	cases := []struct {
		name           string
		peerTags       bool
		primaryTagKeys []string
	}{
		{name: "peer_tags", peerTags: true},
		{name: "primary_only", primaryTagKeys: primary},
		{name: "peer_and_primary", peerTags: true, primaryTagKeys: primary},
		{name: "no_tags"},
	}
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			conf := config.New()
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator
			conf.PeerTagsAggregation = tc.peerTags
			peerTagKeys := conf.ConfiguredPeerTags()
			if tc.peerTags {
				for _, k := range tagLookupPeerAttrs {
					if !slices.Contains(peerTagKeys, k) {
						b.Fatalf("%q is not a configured peer tag", k)
					}
				}
			}
			traces := buildTagLookupTraces(tc.primaryTagKeys)
			b.ReportAllocs()
			for b.Loop() {
				OTLPTracesToConcentratorInputs(traces, conf, nil, peerTagKeys, tc.primaryTagKeys)
			}
		})
	}
}
