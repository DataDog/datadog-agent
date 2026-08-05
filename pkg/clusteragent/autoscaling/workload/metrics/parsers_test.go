// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func newInternalWithObjectMeta(ns, name string, annotations, labels map[string]string) *model.PodAutoscalerInternal {
	internal := model.NewFakePodAutoscalerInternal(ns, name, &model.FakePodAutoscalerInternal{
		UpstreamCR: &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   ns,
				Name:        name,
				Annotations: annotations,
				Labels:      labels,
			},
		},
	})
	return &internal
}

func newInternalWithAnnotations(ns, name string, annotations map[string]string) *model.PodAutoscalerInternal {
	return newInternalWithObjectMeta(ns, name, annotations, nil)
}

func TestKeyTagsFromObjectMetadata(t *testing.T) {
	tests := []struct {
		name         string
		internal     *model.PodAutoscalerInternal
		expectedTags []string
	}{
		{
			name:         "nil internal returns nil",
			internal:     nil,
			expectedTags: nil,
		},
		{
			name: "no upstream CR returns nil",
			internal: func() *model.PodAutoscalerInternal {
				internal := model.NewFakePodAutoscalerInternal("ns", "name", nil)
				return &internal
			}(),
			expectedTags: nil,
		},
		{
			name:         "no annotations and no UST labels returns nil",
			internal:     newInternalWithAnnotations("ns", "name", map[string]string{"other": "value"}),
			expectedTags: nil,
		},
		{
			name:         "empty annotation value and no labels returns nil",
			internal:     newInternalWithAnnotations("ns", "name", map[string]string{"ad.datadoghq.com/tags": ""}),
			expectedTags: nil,
		},
		{
			name:         "single annotation tag",
			internal:     newInternalWithAnnotations("ns", "name", map[string]string{"ad.datadoghq.com/tags": `{"team":"autoscaling"}`}),
			expectedTags: []string{"team:autoscaling"},
		},
		{
			name:         "multiple annotation tags",
			internal:     newInternalWithAnnotations("ns", "name", map[string]string{"ad.datadoghq.com/tags": `{"team":"autoscaling","group":"modern-compute"}`}),
			expectedTags: []string{"team:autoscaling", "group:modern-compute"},
		},
		{
			name:         "invalid JSON falls back to labels only",
			internal:     newInternalWithObjectMeta("ns", "name", map[string]string{"ad.datadoghq.com/tags": `{not valid json`}, map[string]string{"tags.datadoghq.com/env": "prod"}),
			expectedTags: []string{"env:prod"},
		},
		{
			name:         "JSON array value (non-object) returns nil when no labels",
			internal:     newInternalWithAnnotations("ns", "name", map[string]string{"ad.datadoghq.com/tags": `["team:autoscaling"]`}),
			expectedTags: nil,
		},
		{
			name:         "UST labels only",
			internal:     newInternalWithObjectMeta("ns", "name", nil, map[string]string{"tags.datadoghq.com/env": "prod", "tags.datadoghq.com/service": "my-svc", "tags.datadoghq.com/version": "1.0"}),
			expectedTags: []string{"env:prod", "service:my-svc", "version:1.0"},
		},
		{
			name:         "annotations and UST labels combined",
			internal:     newInternalWithObjectMeta("ns", "name", map[string]string{"ad.datadoghq.com/tags": `{"team":"autoscaling"}`}, map[string]string{"tags.datadoghq.com/env": "staging"}),
			expectedTags: []string{"team:autoscaling", "env:staging"},
		},
		{
			name:         "non-UST labels ignored",
			internal:     newInternalWithObjectMeta("ns", "name", nil, map[string]string{"app": "my-app", "other-label": "val"}),
			expectedTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := keyTagsFromObjectMetadata(tt.internal)
			assert.ElementsMatch(t, tt.expectedTags, result)
		})
	}
}

func TestParseContainerAnnotationTags(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    map[string][]string
		expectError bool
	}{
		{
			name:        "nil annotations returns empty map",
			annotations: nil,
			expected:    map[string][]string{},
		},
		{
			name: "resource-level annotation excluded",
			annotations: map[string]string{
				"ad.datadoghq.com/tags": `{"team":"infra"}`,
			},
			expected: map[string][]string{},
		},
		{
			name: "single container annotation",
			annotations: map[string]string{
				"ad.datadoghq.com/app.tags": `{"tier":"frontend"}`,
			},
			expected: map[string][]string{
				"app": {"tier:frontend"},
			},
		},
		{
			name: "multiple container annotations",
			annotations: map[string]string{
				"ad.datadoghq.com/web.tags":    `{"tier":"frontend"}`,
				"ad.datadoghq.com/worker.tags": `{"tier":"backend"}`,
			},
			expected: map[string][]string{
				"web":    {"tier:frontend"},
				"worker": {"tier:backend"},
			},
		},
		{
			name: "resource-level and container annotations mixed",
			annotations: map[string]string{
				"ad.datadoghq.com/tags":     `{"team":"infra"}`,
				"ad.datadoghq.com/app.tags": `{"tier":"frontend"}`,
			},
			expected: map[string][]string{
				"app": {"tier:frontend"},
			},
		},
		{
			name: "invalid JSON returns error",
			annotations: map[string]string{
				"ad.datadoghq.com/app.tags": `{not valid`,
			},
			expected:    map[string][]string{},
			expectError: true,
		},
		{
			name: "unrelated annotations ignored",
			annotations: map[string]string{
				"other.annotation/foo":      "bar",
				"ad.datadoghq.com/app.tags": `{"tier":"frontend"}`,
			},
			expected: map[string][]string{
				"app": {"tier:frontend"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseContainerAnnotationTags(tt.annotations)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, len(tt.expected), len(result))
			for containerName, expectedTags := range tt.expected {
				assert.ElementsMatch(t, expectedTags, result[containerName],
					"tags mismatch for container %q", containerName)
			}
		})
	}
}

func TestParseTagsFromJSON(t *testing.T) {
	tests := []struct {
		name          string
		annotationKey string
		tagsJSON      string
		expectedTags  []string
		expectError   bool
	}{
		{
			name:          "empty string returns nil",
			annotationKey: "ad.datadoghq.com/tags",
			tagsJSON:      "",
			expectedTags:  nil,
		},
		{
			name:          "valid string values",
			annotationKey: "ad.datadoghq.com/tags",
			tagsJSON:      `{"team":"infra","env":"prod"}`,
			expectedTags:  []string{"team:infra", "env:prod"},
		},
		{
			name:          "valid array values",
			annotationKey: "ad.datadoghq.com/tags",
			tagsJSON:      `{"tag":["v1","v2"]}`,
			expectedTags:  []string{"tag:v1", "tag:v2"},
		},
		{
			name:          "invalid JSON returns error",
			annotationKey: "ad.datadoghq.com/tags",
			tagsJSON:      `{not valid json`,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTagsFromJSON(tt.annotationKey, tt.tagsJSON)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else if tt.expectedTags == nil {
				assert.NoError(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedTags, result)
			}
		})
	}
}
