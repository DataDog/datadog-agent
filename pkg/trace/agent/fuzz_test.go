// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package agent

import (
	"reflect"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func FuzzProcessStats(f *testing.F) {
	agent, cancel := agentWithDefaults()
	defer cancel()
	encode := func(pbStats *pb.ClientStatsPayload) ([]byte, error) {
		return pbStats.MarshalVT()
	}
	decode := func(stats []byte) (*pb.ClientStatsPayload, error) {
		payload := &pb.ClientStatsPayload{}
		err := payload.UnmarshalVT(stats)
		return payload, err
	}
	equal := func(x, y *pb.ClientStatsPayload) bool {
		// This function provides a fuzzier comparison than reflect.DeepEqual,
		// allowing for empty slices to be equal using cmpopts.EquateEmpty().
		// cmp.Exporter is an option to compare unexported fields.
		return cmp.Equal(x, y, cmpopts.EquateEmpty(), cmp.Exporter(func(reflect.Type) bool { return true }))
	}
	pbStats := testutil.StatsPayloadSample()
	stats, err := encode(pbStats)
	if err != nil {
		f.Fatalf("Couldn't generate seed corpus: %v", err)
	}
	f.Add(stats, "java", "v1", "abc123")
	f.Fuzz(func(t *testing.T, stats []byte, lang, version, containerID string) {
		pbStats, err := decode(stats)
		if err != nil {
			t.Skipf("Skipping invalid payload: %v", err)
		}
		encPreProcess, err := encode(pbStats)
		if err != nil {
			t.Fatalf("Couldn't encode stats before processing: %v", err)
		}
		decPreProcess, err := decode(encPreProcess)
		if err != nil {
			t.Fatalf("Couldn't decode stats before processing: %v", err)
		}
		if !equal(decPreProcess, pbStats) {
			t.Fatalf("Inconsistent encoding/decoding before processing: (%v) is different from (%v)", decPreProcess, pbStats)
		}
		processedStats := agent.processStats(pbStats, lang, version, containerID, "")
		encPostProcess, err := encode(processedStats)
		if err != nil {
			t.Fatalf("processStats returned an invalid stats payload: %v", err)
		}
		decPostProcess, err := decode(encPostProcess)
		if err != nil {
			t.Fatalf("Couldn't decode stats after processing: %v", err)
		}
		if !equal(decPostProcess, processedStats) {
			t.Fatalf("Inconsistent encoding/decoding after processing: (%v) is different from (%v)", decPostProcess, processedStats)
		}
	})
}

func FuzzObfuscateSpan(f *testing.F) {
	agent, cancel := agentWithDefaults()
	defer cancel()

	// Helper to create InternalSpan from attributes
	createSpan := func(typ, resource string, attrs map[string]string) *idx.InternalSpan {
		st := idx.NewStringTable()
		spanAttrs := make(map[uint32]*idx.AnyValue)
		for k, v := range attrs {
			spanAttrs[st.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: st.Add(v),
				},
			}
		}
		return idx.NewInternalSpan(st, &idx.Span{
			TypeRef:     st.Add(typ),
			ResourceRef: st.Add(resource),
			Attributes:  spanAttrs,
		})
	}

	encode := func(span *idx.InternalSpan) ([]byte, error) {
		serStrings := idx.NewSerializedStrings(uint32(span.Strings.Len()))
		return span.MarshalMsg(nil, serStrings)
	}

	decode := func(data []byte, st *idx.StringTable) (*idx.InternalSpan, error) {
		span := idx.NewInternalSpan(st, &idx.Span{})
		_, err := span.UnmarshalMsg(data)
		return span, err
	}

	// Seed corpus with InternalSpan structures
	seedCorpus := []*idx.InternalSpan{
		createSpan("redis", "SET k v\nGET k", map[string]string{"redis.raw_command": "SET k v\nGET k"}),
		createSpan("valkey", "SET k v\nGET k", map[string]string{"valkey.raw_command": "SET k v\nGET k"}),
		createSpan("sql", "UPDATE users(name) SET ('Jim')", map[string]string{"sql.query": "UPDATE users(name) SET ('Jim')"}),
		createSpan("http", "http://mysite.mydomain/1/2?q=asd", map[string]string{"http.url": "http://mysite.mydomain/1/2?q=asd"}),
	}

	for _, span := range seedCorpus {
		encoded, err := encode(span)
		if err != nil {
			f.Fatalf("Couldn't generate seed corpus: %v", err)
		}
		f.Add(encoded)
	}

	f.Fuzz(func(t *testing.T, spanData []byte) {
		st := idx.NewStringTable()
		span, err := decode(spanData, st)
		if err != nil {
			t.Skipf("Skipping invalid span: %v", err)
		}

		// Obfuscate the span
		agent.obfuscateSpanInternal(span)

		// Encode after obfuscation
		encPostObfuscate, err := encode(span)
		if err != nil {
			t.Fatalf("obfuscateSpanInternal returned an invalid span: %v", err)
		}

		// Decode again to verify consistency
		st2 := idx.NewStringTable()
		decPostObfuscate, err := decode(encPostObfuscate, st2)
		if err != nil {
			t.Fatalf("Couldn't decode span after obfuscation: %v", err)
		}

		// Compare the obfuscated spans
		if span.Resource() != decPostObfuscate.Resource() {
			t.Fatalf("Inconsistent Resource after encoding/decoding: %q vs %q", span.Resource(), decPostObfuscate.Resource())
		}
		if span.Type() != decPostObfuscate.Type() {
			t.Fatalf("Inconsistent Type after encoding/decoding: %q vs %q", span.Type(), decPostObfuscate.Type())
		}
	})
}

func FuzzNormalizeTrace(f *testing.F) {
	agent, cancel := agentWithDefaults()
	defer cancel()
	encode := func(pbTrace pb.Trace) ([]byte, error) {
		return pbTrace.MarshalMsg(nil)
	}
	decode := func(trace []byte) (pb.Trace, error) {
		var pbTrace pb.Trace
		_, err := pbTrace.UnmarshalMsg(trace)
		return pbTrace, err
	}
	pbTrace := pb.Trace{newTestSpan(), newTestSpan()}
	trace, err := encode(pbTrace)
	if err != nil {
		f.Fatalf("Couldn't generate seed corpus: %v", err)
	}
	f.Add(trace)
	f.Fuzz(func(t *testing.T, trace []byte) {
		pbTrace, err := decode(trace)
		if err != nil {
			t.Skipf("Skipping invalid trace: %v", err)
		}
		ts := newTagStats()
		if err := agent.normalizeTrace(ts, pbTrace); err != nil {
			t.Skipf("Skipping rejected trace: %v", err)
		}
		encPostNorm, err := encode(pbTrace)
		if err != nil {
			t.Fatalf("normalizeTrace returned an invalid trace: %v", err)
		}
		decPostNorm, err := decode(encPostNorm)
		if err != nil {
			t.Fatalf("Couldn't decode trace after normalization: %v", err)
		}
		if !reflect.DeepEqual(decPostNorm, pbTrace) {
			t.Fatalf("Inconsistent encoding/decoding after normalization: (%#v) is different from (%#v)", decPostNorm, pbTrace)
		}
	})
}
