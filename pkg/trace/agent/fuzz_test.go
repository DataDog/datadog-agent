// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.18

package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func FuzzProcessStats(f *testing.F) {
	agent, cancel := agentWithDefaults()
	defer cancel()
	encode := func(pbStats pb.ClientStatsPayload) ([]byte, error) {
		return pbStats.Marshal()
	}
	decode := func(stats []byte) (pb.ClientStatsPayload, error) {
		var payload pb.ClientStatsPayload
		err := payload.Unmarshal(stats)
		return payload, err
	}
	pbStats := testutil.StatsPayloadSample()
	stats, err := encode(pbStats)
	if err != nil {
		f.Fatal("Couldn't generate seed corpus:", err)
	}
	f.Add(stats, "java", "v1")
	f.Fuzz(func(t *testing.T, stats []byte, lang, version string) {
		pbStats, err := decode(stats)
		if err != nil {
			t.Skip("Skipping invalid payload:", err)
		}
		processedStats := agent.processStats(pbStats, lang, version)
		if _, err = encode(processedStats); err != nil {
			t.Fatal("processStats returned an invalid stats payload:", err)
		}
	})
}

func FuzzObfuscateSpan(f *testing.F) {
	agent, cancel := agentWithDefaults()
	defer cancel()
	encode := func(pbSpan *pb.Span) ([]byte, error) {
		return pbSpan.Marshal()
	}
	decode := func(span []byte) (pb.Span, error) {
		var pbSpan pb.Span
		err := pbSpan.Unmarshal(span)
		return pbSpan, err
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
			f.Fatal("Couldn't generate seed corpus:", err)
		}
		f.Add(span)
	}
	f.Fuzz(func(t *testing.T, span []byte) {
		pbSpan, err := decode(span)
		if err != nil {
			t.Skip("Skipping invalid span:", err)
		}
		agent.obfuscateSpan(&pbSpan)
		if _, err = encode(&pbSpan); err != nil {
			t.Fatal("obfuscateSpan returned an invalid span:", err)
		}
	})
}

func FuzzNormalizeTrace(f *testing.F) {
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
		f.Fatal("Couldn't generate seed corpus:", err)
	}
	f.Add(trace)
	f.Fuzz(func(t *testing.T, trace []byte) {
		pbTrace, err := decode(trace)
		if err != nil {
			t.Skip("Skipping invalid trace:", err)
		}
		ts := newTagStats()
		if err := normalizeTrace(ts, pbTrace); err != nil {
			t.Skip("Skipping rejected trace:", err)
		}
		if _, err = encode(pbTrace); err != nil {
			t.Fatal("normalizeTrace returned an invalid trace:", err)
		}
	})
}
