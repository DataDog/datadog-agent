// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveChunk(t *testing.T) {
	tp := &TracerPayload{
		Chunks: []*TraceChunk{
			{Origin: "chunk-0"},
			{Origin: "chunk-1"},
			{Origin: "chunk-2"},
		},
	}
	tp.RemoveChunk(3)  // does nothing
	tp.RemoveChunk(-1) // does nothing
	tp.RemoveChunk(2)
	tp.RemoveChunk(0)
	assert.Len(t, tp.Chunks, 1)
	assert.Equal(t, tp.Chunks[0].Origin, "chunk-1")
}

func TestCut(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		tp := &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		}
		tp1 := tp.Cut(1)
		assert.Equal(t, tp1, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
			},
		})
		assert.Equal(t, tp, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		})
	})

	t.Run("lower-boundary", func(t *testing.T) {
		tp := &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		}
		tp1 := tp.Cut(-1)
		assert.Equal(t, tp1, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks:          []*TraceChunk{},
		})
		assert.Equal(t, tp, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		})
	})

	t.Run("upper-boundary", func(t *testing.T) {
		tp := &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		}
		tp1 := tp.Cut(100)
		assert.Equal(t, tp1, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks: []*TraceChunk{
				{Origin: "chunk-0"},
				{Origin: "chunk-1"},
				{Origin: "chunk-2"},
			},
		})
		assert.Equal(t, tp, &TracerPayload{
			Tags: map[string]string{
				"_dd.container_tags": "kube_deployment:trace-usage-tracker",
			},
			LanguageName:    "python",
			LanguageVersion: "3.8.1",
			TracerVersion:   "1.2.3",
			ContainerID:     "abcdef123789",
			Chunks:          []*TraceChunk{},
		})
	})
}
