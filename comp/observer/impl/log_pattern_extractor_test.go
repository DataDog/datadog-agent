// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogPatternExtractor_GetContextByKeyUsesOutputContextKey(t *testing.T) {
	e := NewLogPatternExtractor()

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		tags:    []string{"service:web", "env:prod"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res, 1)
	require.NotEmpty(t, res[0].ContextKey)

	ctx, ok := e.GetContextByKey(res[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, "log_pattern_extractor", ctx.Source)
	assert.Equal(t, "GET /users/123 returned 500", ctx.Example)
	assert.NotEmpty(t, ctx.Pattern)
}

func TestLogPatternExtractor_ContextKeySeparatesSameMetricByTags(t *testing.T) {
	e := NewLogPatternExtractor()

	logA := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: []byte("GET /users/456 returned 500"),
		tags:    []string{"service:worker"},
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	require.Len(t, resA, 1)
	require.Len(t, resB, 1)
	require.Equal(t, resA[0].Name, resB[0].Name)
	require.NotEqual(t, resA[0].ContextKey, resB[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB[0].ContextKey)
	require.True(t, ok)

	assert.Equal(t, "GET /users/123 returned 500", ctxA.Example)
	assert.Equal(t, "GET /users/456 returned 500", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
}

func TestLogPatternExtractor_ResetClearsContext(t *testing.T) {
	e := NewLogPatternExtractor()

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		tags:    []string{"service:web"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res, 1)

	_, ok := e.GetContextByKey(res[0].ContextKey)
	require.True(t, ok)

	e.Reset()

	_, ok = e.GetContextByKey(res[0].ContextKey)
	assert.False(t, ok)
}
