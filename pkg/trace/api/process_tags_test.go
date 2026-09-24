// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

func TestFilterProcessTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "nothing to filter",
			input:    "entrypoint.name:com.example.Main,entrypoint.type:class,svc.auto:demo",
			expected: "entrypoint.name:com.example.Main,entrypoint.type:class,svc.auto:demo",
		},
		{
			name:     "workdir and basedir removed",
			input:    "entrypoint.basedir:exe,entrypoint.name:gotrace,entrypoint.type:executable,entrypoint.workdir:gotrace",
			expected: "entrypoint.name:gotrace,entrypoint.type:executable",
		},
		{
			name:     "only high cardinality tags",
			input:    "entrypoint.workdir:release-20260924,entrypoint.basedir:bin",
			expected: "",
		},
		{
			name:     "keys with surrounding spaces",
			input:    " entrypoint.workdir :app,entrypoint.name:app",
			expected: "entrypoint.name:app",
		},
		{
			name:     "similar key prefix is kept",
			input:    "entrypoint.workdir_hint:app",
			expected: "entrypoint.workdir_hint:app",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, filterProcessTags(tc.input))
		})
	}
}

func TestFilterSpanProcessTags(t *testing.T) {
	tp := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{Spans: []*pb.Span{{Meta: map[string]string{tagProcessTags: "entrypoint.name:app,entrypoint.workdir:app"}}}},
			{Spans: []*pb.Span{{Meta: map[string]string{tagProcessTags: "entrypoint.basedir:bin"}}}},
			{Spans: []*pb.Span{{Meta: map[string]string{"foo": "bar"}}}},
			{},
		},
	}

	filterSpanProcessTags(tp)

	assert.Equal(t, "entrypoint.name:app", tp.Chunks[0].Spans[0].Meta[tagProcessTags])
	assert.NotContains(t, tp.Chunks[1].Spans[0].Meta, tagProcessTags)
	assert.Equal(t, map[string]string{"foo": "bar"}, tp.Chunks[2].Spans[0].Meta)
}

func TestFilterSpanProcessTagsV1(t *testing.T) {
	strings := idx.NewStringTable()
	kept := idx.NewInternalSpan(strings, &idx.Span{})
	kept.SetStringAttribute(tagProcessTags, "entrypoint.name:app,entrypoint.workdir:app")
	removed := idx.NewInternalSpan(strings, &idx.Span{})
	removed.SetStringAttribute(tagProcessTags, "entrypoint.basedir:bin")
	tp := &idx.InternalTracerPayload{
		Strings: strings,
		Chunks: []*idx.InternalTraceChunk{
			idx.NewInternalTraceChunk(strings, 0, "", nil, []*idx.InternalSpan{kept}, false, nil, 0),
			idx.NewInternalTraceChunk(strings, 0, "", nil, []*idx.InternalSpan{removed}, false, nil, 0),
			idx.NewInternalTraceChunk(strings, 0, "", nil, nil, false, nil, 0),
		},
	}

	filterSpanProcessTagsV1(tp)

	ptags, ok := kept.GetAttributeAsString(tagProcessTags)
	assert.True(t, ok)
	assert.Equal(t, "entrypoint.name:app", ptags)
	_, ok = removed.GetAttributeAsString(tagProcessTags)
	assert.False(t, ok)
}

func TestHandleTracesFiltersProcessTags(t *testing.T) {
	const expected = "entrypoint.name:app"
	newRequest := func(t *testing.T) *http.Request {
		tp := &pb.TracerPayload{
			Chunks: []*pb.TraceChunk{
				{Spans: []*pb.Span{{
					Service: "svc",
					Name:    "op",
					TraceID: 1,
					SpanID:  1,
					Meta:    map[string]string{tagProcessTags: "entrypoint.name:app,entrypoint.workdir:release-20260924"},
				}}},
			},
		}
		bts, err := tp.MarshalMsg(nil)
		require.NoError(t, err)
		req, _ := http.NewRequest("POST", "/v0.7/traces", bytes.NewReader(bts))
		req.Header.Set("Content-Type", "application/msgpack")
		return req
	}

	t.Run("v1", func(t *testing.T) {
		receiver := newTestReceiverFromConfig(newTestReceiverConfig())
		rr := httptest.NewRecorder()
		receiver.handleWithVersion(V07, receiver.handleTraces).ServeHTTP(rr, newRequest(t))
		require.Equal(t, http.StatusOK, rr.Code)

		select {
		case p := <-receiver.outV1:
			assert.Equal(t, expected, p.ProcessTags)
			ptags, _ := p.TracerPayload.GetAttributeAsString(tagProcessTags)
			assert.Equal(t, expected, ptags)
			ptags, _ = p.TracerPayload.Chunks[0].Spans[0].GetAttributeAsString(tagProcessTags)
			assert.Equal(t, expected, ptags)
		default:
			t.Fatal("no trace sent for processing")
		}
	})

	t.Run("legacy", func(t *testing.T) {
		receiver := newTestReceiverFromConfig(newTestReceiverConfigWithFeatures("disable-convert-traces"))
		rr := httptest.NewRecorder()
		receiver.handleWithVersion(V07, receiver.handleTraces).ServeHTTP(rr, newRequest(t))
		require.Equal(t, http.StatusOK, rr.Code)

		select {
		case p := <-receiver.out:
			assert.Equal(t, expected, p.ProcessTags)
			assert.Equal(t, expected, p.TracerPayload.Tags[tagProcessTags])
			assert.Equal(t, expected, p.TracerPayload.Chunks[0].Spans[0].Meta[tagProcessTags])
		default:
			t.Fatal("no trace sent for processing")
		}
	})
}

func TestHandleStatsFiltersProcessTags(t *testing.T) {
	receiver := newTestReceiverFromConfig(newTestReceiverConfig())
	processor := &mockStatsProcessor{}
	receiver.statsProcessor = processor

	var buf bytes.Buffer
	payload := &pb.ClientStatsPayload{
		Hostname:    "host",
		ProcessTags: "entrypoint.basedir:bin,entrypoint.name:app",
	}
	require.NoError(t, msgp.Encode(&buf, payload))

	req, _ := http.NewRequest("POST", "/v0.6/stats", &buf)
	req.Header.Set("Content-Type", "application/msgpack")
	rr := httptest.NewRecorder()
	receiver.handleStats(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	processor.mu.Lock()
	defer processor.mu.Unlock()
	require.NotNil(t, processor.lastP)
	assert.Equal(t, "entrypoint.name:app", processor.lastP.ProcessTags)
}
