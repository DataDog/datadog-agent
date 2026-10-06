// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashKey(t *testing.T) {
	tests := []struct {
		name      string
		criteria  []string
		namespace string
		resource  string
		expected  HashKey
	}{
		{
			name:      "resource",
			criteria:  []string{CriterionResource},
			namespace: "datadog",
			resource:  "pod",
			expected:  "|pod",
		},
		{
			name:      "namespace",
			criteria:  []string{CriterionNamespace},
			namespace: "datadog",
			resource:  "pod",
			expected:  "datadog",
		},
		{
			name:      "namespace and resource in reverse order",
			criteria:  []string{CriterionResource, CriterionNamespace},
			namespace: "datadog",
			resource:  "pod",
			expected:  "datadog|pod",
		},
		{
			name:      "no criteria",
			namespace: "datadog",
			resource:  "pod",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := NewHashKey(tt.criteria, tt.namespace, tt.resource)
			assert.Equal(t, tt.expected, key, "unexpected hash key generated")
		})
	}
}

func TestScoreGoldenValues(t *testing.T) {
	tests := []struct {
		name     string
		key      HashKey
		shard    int
		expected uint64
	}{
		{name: "namespace", key: "datadog", shard: 0, expected: 4406660298979129024},
		{name: "resource", key: "|pods", shard: 4, expected: 9733342946436970124},
		{name: "namespace and resource", key: "datadog|pods", shard: 9, expected: 16647598903112153968},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, score(tt.key, tt.shard))
		})
	}
}

func TestShardResponsibleForKeyGoldenValues(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		criteria   []string
		namespace  string
		resource   string
		expectedID int
	}{
		{name: "namespace", count: 3, criteria: []string{CriterionNamespace}, namespace: "datadog", resource: "pods", expectedID: 2},
		{name: "resource", count: 10, criteria: []string{CriterionResource}, namespace: "datadog", resource: "pods", expectedID: 7},
		{name: "namespace and resource", count: 10, criteria: []string{CriterionResource, CriterionNamespace}, namespace: "datadog", resource: "pods", expectedID: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := NewHashKey(tt.criteria, tt.namespace, tt.resource)
			assert.Equal(t, tt.expectedID, ShardResponsibleForKey(tt.count, key))
		})
	}
}
