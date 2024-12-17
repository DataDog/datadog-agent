// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreshSpan(t *testing.T) {
	ctx := context.Background()
	_, ok := SpanFromContext(ctx)
	require.False(t, ok)

	s, ctx := StartSpanFromContext(ctx, "test")
	require.NotNil(t, s)
	s.SetResourceName("new")

	span, ok := SpanFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, s, span)

	assert.Equal(t, "test", s.span.Name)
	assert.Equal(t, "new", s.span.Resource)
	assert.Equal(t, "new", s.span.Resource)
	assert.Equal(t, "new", span.span.Resource)
}

func TestInheritence(t *testing.T) {
	ctx := context.Background()
	s, ctx := StartSpanFromContext(ctx, "test")
	require.NotNil(t, s)

	child, _ := StartSpanFromContext(ctx, "child")
	require.NotNil(t, child)

	assert.Equal(t, s.span.SpanID, child.span.ParentID)
	assert.Equal(t, s.span.TraceID, child.span.TraceID)
}

func TestStartSpanFromIDs(t *testing.T) {
	ctx := context.Background()
	traceID := "100"
	parentID := "200"

	span, ctx := StartSpanFromIDs(ctx, "ids-operation", traceID, parentID)
	require.NotNil(t, span, "Expected a span")
	require.Equal(t, uint64(100), span.span.TraceID)
	require.Equal(t, uint64(200), span.span.ParentID)

	val, ok := span.span.Metrics["_top_level"]
	require.True(t, ok)
	require.Equal(t, 1.0, val)

	spanFromCtx, ok := SpanFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, span, spanFromCtx)
}
