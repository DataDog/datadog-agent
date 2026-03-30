// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogPatternExtractor_GetContextByKeyUsesOutputContextKey(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:web", "env:prod"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	require.NotEmpty(t, res.Metrics[0].ContextKey)

	ctx, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, "log_pattern_extractor", ctx.Source)
	assert.Equal(t, "GET /users/123 returned 500", ctx.Example)
	assert.NotEmpty(t, ctx.Pattern)
}

func TestLogPatternExtractor_ContextKeySeparatesSameMetricByTags(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	logA := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: []byte("GET /users/456 returned 500"),
		status:  "warn",
		tags:    []string{"service:worker"},
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	require.Equal(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)

	assert.Equal(t, "GET /users/123 returned 500", ctxA.Example)
	assert.Equal(t, "GET /users/456 returned 500", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
}

func TestLogPatternExtractor_ResetClearsContext(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:web"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	_, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)

	e.Reset()

	_, ok = e.GetContextByKey(res.Metrics[0].ContextKey)
	assert.False(t, ok)
}

func TestLogPatternExtractor_SkipsBelowWarnSeverity(t *testing.T) {
	e := NewLogPatternExtractor()

	out := e.ProcessLog(&mockLogView{
		content: []byte("INFO: routine request completed"),
		status:  "info",
		tags:    []string{"service:api"},
	})
	require.Empty(t, out.Metrics)
	require.Empty(t, out.Telemetry)
}

func TestLogPatternExtractor_DeferredEmitUntilMinPatterns(t *testing.T) {
	e := NewLogPatternExtractor()
	status := "warn"
	tags := []string{"service:api"}

	for i := range 4 {
		out := e.ProcessLog(&mockLogView{
			content: []byte(fmt.Sprintf("WARN distinct pattern seed %d not mergeable xyz", i)),
			status:  status,
			tags:    tags,
		})
		require.Empty(t, out.Metrics, "i=%d", i)
	}

	out := e.ProcessLog(&mockLogView{
		content: []byte("WARN distinct pattern seed 4 not mergeable xyz"),
		status:  status,
		tags:    tags,
	})
	require.Len(t, out.Metrics, 1)
}
