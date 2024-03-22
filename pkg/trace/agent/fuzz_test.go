// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.18 && !windows

package agent

import (
	"reflect"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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
	f.Add(stats, "java", "v1")
	f.Fuzz(func(t *testing.T, stats []byte, lang, version string) {
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
		processedStats := agent.processStats(pbStats, lang, version)
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
	encode := func(pbSpan *pb.Span) ([]byte, error) {
		return pbSpan.MarshalMsg(nil)
	}
	decode := func(span []byte) (*pb.Span, error) {
		var pbSpan pb.Span
		_, err := pbSpan.UnmarshalMsg(span)
		return &pbSpan, err
	}
	seedCorpus := []*pb.Span{
		{
			Type:     "redis",
			Resource: "SET k v\nGET k",
			Meta:     map[string]string{"redis.raw_command": "SET k v\nGET k"},
		},
		{
			Type:     "sql",
			Resource: "UPDATE users(name) SET ('Jim')",
			Meta:     map[string]string{"sql.query": "UPDATE users(name) SET ('Jim')"},
		},
		{
			Type:     "http",
			Resource: "http://mysite.mydomain/1/2?q=asd",
			Meta:     map[string]string{"http.url": "http://mysite.mydomain/1/2?q=asd"},
		},
	}
	for _, span := range seedCorpus {
		span, err := encode(span)
		if err != nil {
			f.Fatalf("Couldn't generate seed corpus: %v", err)
		}
		f.Add(span)
	}
	f.Fuzz(func(t *testing.T, span []byte) {
		pbSpan, err := decode(span)
		if err != nil {
			t.Skipf("Skipping invalid span: %v", err)
		}
		agent.obfuscateSpan(pbSpan)
		encPostObfuscate, err := encode(pbSpan)
		if err != nil {
			t.Fatalf("obfuscateSpan returned an invalid span: %v", err)
		}
		decPostObfuscate, err := decode(encPostObfuscate)
		if err != nil {
			t.Fatalf("Couldn't decode span after obfuscation: %v", err)
		}
		if !reflect.DeepEqual(decPostObfuscate, pbSpan) {
			t.Fatalf("Inconsistent encoding/decoding after obfuscation: (%#v) is different from (%#v)", decPostObfuscate, pbSpan)
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
