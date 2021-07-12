// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getStandardTags(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name: "nominal case",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
				"tags.datadoghq.com/env":       "prod",
				"tags.datadoghq.com/version":   "1.5.2",
				"tags.datadoghq.com/service":   "datadog",
			},
			want: []string{
				"env:prod",
				"version:1.5.2",
				"service:datadog",
			},
		},
		{
			name:   "nil input",
			labels: nil,
			want:   []string{},
		},
		{
			name: "standard tags not found",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
			},
			want: []string{},
		},
		{
			name: "standard tags not all found",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
				"tags.datadoghq.com/env":       "prod",
				"tags.datadoghq.com/service":   "datadog",
			},
			want: []string{
				"env:prod",
				"service:datadog",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStandardTags(tt.labels)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func Test_standardTagsDigest(t *testing.T) {
	tests := []struct {
		name      string
		first     map[string]string
		second    map[string]string
		equalHash bool
	}{
		{
			name: "same standard tags",
			first: map[string]string{
				"app.kubernetes.io/instance": "cluster-agent",
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/version": "1.5.2",
				"tags.datadoghq.com/service": "datadog",
			},
			second: map[string]string{
				"tags.datadoghq.com/version": "1.5.2",
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/service": "datadog",
			},
			equalHash: true,
		},
		{
			name: "1 standard deleted",
			first: map[string]string{
				"app.kubernetes.io/instance": "cluster-agent",
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/version": "1.5.2",
			},
			second: map[string]string{
				"tags.datadoghq.com/version": "1.5.2",
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/service": "datadog",
			},
			equalHash: false,
		},
		{
			name: "1 standard changed value",
			first: map[string]string{
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/version": "1.5.2",
				"tags.datadoghq.com/service": "datadog",
			},
			second: map[string]string{
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/version": "1.5.3",
				"tags.datadoghq.com/service": "datadog",
			},
			equalHash: false,
		},
		{
			name: "no standard tags",
			first: map[string]string{
				"app.kubernetes.io/instance": "cluster-agent",
			},
			second:    map[string]string{},
			equalHash: true,
		},
		{
			name:      "no labels",
			first:     map[string]string{},
			second:    map[string]string{},
			equalHash: true,
		},
		{
			name:  "nil labels",
			first: nil,
			second: map[string]string{
				"tags.datadoghq.com/service": "datadog",
			},
			equalHash: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := standardTagsDigest(tt.first)
			second := standardTagsDigest(tt.second)
			if (first == second) != tt.equalHash {
				t.Errorf("hash1: %s, hash2: %s, want: %v", first, second, tt.equalHash)
			}
		})
	}
}
