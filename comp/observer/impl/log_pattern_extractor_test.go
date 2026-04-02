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
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

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
	assert.Equal(t, map[string]string{"service": "web", "env": "prod"}, ctx.SplitTags)
}

func TestLogPatternExtractor_DifferentTagGroupsProduceDifferentMetricNames(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	// 1 pattern per service (same pattern strings but different IDs)
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
	logC := &mockLogView{
		content: []byte("GET /users/124 returned 500"),
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logD := &mockLogView{
		content: []byte("GET /users/457 returned 500"),
		status:  "warn",
		tags:    []string{"service:worker"},
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	e.ProcessLog(logC)
	e.ProcessLog(logD)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	// Different tag groups → different sub-clusterers → different globalClusterHash → different names.
	require.NotEqual(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)

	assert.Equal(t, "GET /users/123 returned 500", ctxA.Example)
	assert.Equal(t, "GET /users/456 returned 500", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
	assert.Equal(t, map[string]string{"service": "api"}, ctxA.SplitTags)
	assert.Equal(t, map[string]string{"service": "worker"}, ctxB.SplitTags)
}

func TestLogPatternExtractor_DifferentHostnamesProduceDifferentMetricNamesWhenNoHostTag(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	msg := []byte("GET /users/123 returned 500")
	tags := []string{"service:api", "env:prod"}

	logA := &mockLogView{
		content:  msg,
		status:   "warn",
		tags:     tags,
		hostname: "host-a",
	}
	logB := &mockLogView{
		content:  msg,
		status:   "warn",
		tags:     tags,
		hostname: "host-b",
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	require.NotEqual(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-a"}, ctxA.SplitTags)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-b"}, ctxB.SplitTags)
}

func TestLogPatternExtractor_ResetClearsContext(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

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
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())

	out := e.ProcessLog(&mockLogView{
		content: []byte("INFO: routine request completed"),
		status:  "info",
		tags:    []string{"service:api"},
	})
	require.Empty(t, out.Metrics)
	require.Empty(t, out.Telemetry)
}

func TestLogPatternExtractor_DeferredEmitUntilMinPatterns(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
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
