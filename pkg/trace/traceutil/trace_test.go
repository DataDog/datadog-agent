// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/version"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func TestGetRootFromCompleteTrace(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12341), Service: "s1", Name: "n1", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12342), ParentID: uint64(12341), Service: "s1", Name: "n1", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12343), ParentID: uint64(12341), Service: "s1", Name: "n1", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12344), ParentID: uint64(12342), Service: "s2", Name: "n2", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12345), ParentID: uint64(12344), Service: "s2", Name: "n2", Resource: ""},
	}

	assert.Equal(GetRoot(trace).SpanID, uint64(12341))
}

func TestGetRootFromPartialTrace(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12341), ParentID: uint64(12340), Service: "s1", Name: "n1", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12342), ParentID: uint64(12341), Service: "s1", Name: "n1", Resource: ""},
		&pb.Span{TraceID: uint64(1234), SpanID: uint64(12343), ParentID: uint64(12342), Service: "s2", Name: "n2", Resource: ""},
	}

	assert.Equal(GetRoot(trace).SpanID, uint64(12341))
}

func TestTraceChildrenMap(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{SpanID: 1, ParentID: 0},
		&pb.Span{SpanID: 2, ParentID: 1},
		&pb.Span{SpanID: 3, ParentID: 1},
		&pb.Span{SpanID: 4, ParentID: 2},
		&pb.Span{SpanID: 5, ParentID: 3},
		&pb.Span{SpanID: 6, ParentID: 4},
	}

	childrenMap := ChildrenMap(trace)

	assert.Equal([]*pb.Span{trace[1], trace[2]}, childrenMap[1])
	assert.Equal([]*pb.Span{trace[3]}, childrenMap[2])
	assert.Equal([]*pb.Span{trace[4]}, childrenMap[3])
	assert.Equal([]*pb.Span{trace[5]}, childrenMap[4])
	assert.Equal([]*pb.Span(nil), childrenMap[5])
	assert.Equal([]*pb.Span(nil), childrenMap[6])
}

func TestGetEnv(t *testing.T) {
	tts := []struct {
		name     string
		in       pb.Trace
		expected string
	}{
		{
			name: "no-env",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0},
			},
			expected: "",
		},
		{
			name: "root_env",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0, Meta: map[string]string{"env": "root"}},
			},
			expected: "root",
		},
		{
			name: "env",
			in: pb.Trace{
				&pb.Span{SpanID: 24, ParentID: 5, Meta: map[string]string{"env": "env"}},
				&pb.Span{ParentID: 0},
			},
			expected: "env",
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetEnv(GetRoot(tc.in), &pb.TraceChunk{Spans: tc.in}))
		})
	}
}

func TestGetAppVersion(t *testing.T) {
	tts := []struct {
		name     string
		in       pb.Trace
		expected string
	}{
		{
			name: "no-version",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0},
			},
			expected: "",
		},
		{
			name: "root_ver",
			in: pb.Trace{
				&pb.Span{ParentID: 5},
				&pb.Span{ParentID: 0, Meta: map[string]string{"version": "root_ver"}},
			},
			expected: "root_ver",
		},
		{
			name: "version",
			in: pb.Trace{
				&pb.Span{SpanID: 24, ParentID: 5, Meta: map[string]string{"version": "version"}},
				&pb.Span{ParentID: 0},
			},
			expected: "version",
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, version.GetAppVersionFromTrace(GetRoot(tc.in), &pb.TraceChunk{Spans: tc.in}))
		})
	}
}
