// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/stretchr/testify/assert"
)

func TestClone(t *testing.T) {
	root := &pb.Span{Name: "root"}
	span1 := &pb.Span{Name: "span1"}
	span2 := &pb.Span{Name: "span2"}
	spans := []*pb.Span{root, span1, span2}
	pt := &ProcessedTrace{
		TraceChunk: &pb.TraceChunk{Spans: spans},
		Root:       root,
	}

	clone := pt.Clone()

	clone.TraceChunk = &pb.TraceChunk{Spans: []*pb.Span{root, span2}} // remove span1
	assert.Len(t, clone.TraceChunk.Spans, 2)
	assert.Len(t, pt.TraceChunk.Spans, 3) // TraceChunk in pt shouldn't change

	clone.Root.Name = "root-changed"
	assert.Equal(t, "root-changed", clone.Root.Name)
	assert.Equal(t, "root", pt.Root.Name) // Root in pt shouldn't change

	clone.TraceChunk.Spans[1].Name = "span2-changed"
	assert.Equal(t, "span2-changed", clone.TraceChunk.Spans[1].Name)
	assert.Equal(t, "span2-changed", pt.TraceChunk.Spans[2].Name) // span2 in pt should change as we're doing a semi-deep copy
}
