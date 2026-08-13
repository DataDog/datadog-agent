// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeTags(t *testing.T) {
	tests := []struct {
		name    string
		tagSets [][]string
		want    []string
	}{
		{
			name: "no tag sets",
		},
		{
			name:    "empty and nil tag sets",
			tagSets: [][]string{nil, {}},
		},
		{
			name:    "single tag set preserves order",
			tagSets: [][]string{{"team:payments", "env:prod"}},
			want:    []string{"team:payments", "env:prod"},
		},
		{
			name:    "multiple tag sets preserve first-seen order",
			tagSets: [][]string{{"team:payments", "env:prod"}, {"service:checkout", "region:us-east-1"}},
			want:    []string{"team:payments", "env:prod", "service:checkout", "region:us-east-1"},
		},
		{
			name:    "duplicates within one tag set are removed",
			tagSets: [][]string{{"env:prod", "env:prod", "team:payments"}},
			want:    []string{"env:prod", "team:payments"},
		},
		{
			name:    "duplicates across tag sets are removed",
			tagSets: [][]string{{"team:payments", "env:prod"}, {"env:prod", "service:checkout"}},
			want:    []string{"team:payments", "env:prod", "service:checkout"},
		},
		{
			name:    "distinct values for the same key are preserved",
			tagSets: [][]string{{"env:prod"}, {"env:staging"}},
			want:    []string{"env:prod", "env:staging"},
		},
		{
			name:    "empty tag strings follow exact-string semantics",
			tagSets: [][]string{{"", "team:payments"}, {""}},
			want:    []string{"", "team:payments"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mergeTags(tt.tagSets...))
		})
	}
}
