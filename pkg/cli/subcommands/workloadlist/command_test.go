// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadlist

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadURL_NoParams(t *testing.T) {
	url, err := workloadURL(false, false, "")
	require.NoError(t, err)
	// Should not have query parameters
	assert.NotContains(t, url, "?")
	assert.Contains(t, url, "/agent/workload-list")
}

func TestWorkloadURL_SingleParam(t *testing.T) {
	tests := []struct {
		name      string
		verbose   bool
		format    bool
		search    string
		contains  string
		notContains []string
	}{
		{
			name:        "verbose only",
			verbose:     true,
			contains:    "?verbose=true",
			notContains: []string{"format=", "search="},
		},
		{
			name:        "format only",
			format:      true,
			contains:    "?format=json",
			notContains: []string{"verbose=", "search="},
		},
		{
			name:        "search only",
			search:      "container",
			contains:    "?search=container",
			notContains: []string{"verbose=", "format="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := workloadURL(tt.verbose, tt.format, tt.search)
			require.NoError(t, err)
			assert.Contains(t, url, tt.contains)
			for _, nc := range tt.notContains {
				assert.NotContains(t, url, nc)
			}
		})
	}
}

func TestWorkloadURL_TwoParams(t *testing.T) {
	tests := []struct {
		name      string
		verbose   bool
		format    bool
		search    string
		contains  []string
	}{
		{
			name:     "verbose and search",
			verbose:  true,
			search:   "test",
			contains: []string{"verbose=true", "search=test", "&"},
		},
		{
			name:     "format and search",
			format:   true,
			search:   "test",
			contains: []string{"format=json", "search=test", "&"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := workloadURL(tt.verbose, tt.format, tt.search)
			require.NoError(t, err)
			for _, c := range tt.contains {
				assert.Contains(t, url, c)
			}
		})
	}
}

func TestWorkloadURL_AllParams(t *testing.T) {
	url, err := workloadURL(true, true, "pod")
	require.NoError(t, err)
	assert.Contains(t, url, "verbose=true")
	assert.Contains(t, url, "format=json")
	assert.Contains(t, url, "search=pod")
	// Check that parameters are properly joined with &
	assert.Contains(t, url, "?")
	assert.Contains(t, url, "&")
}

func TestWorkloadURL_EmptySearchIsOmitted(t *testing.T) {
	url, err := workloadURL(true, true, "")
	require.NoError(t, err)
	// Empty search should not be included in URL
	assert.NotContains(t, url, "search=")
	assert.Contains(t, url, "verbose=true")
	assert.Contains(t, url, "format=json")
}

func TestWorkloadURL_SearchWithSpecialChars(t *testing.T) {
	tests := []struct {
		name   string
		search string
	}{
		{"simple text", "my-container-123"},
		{"kubernetes name", "kubernetes_pod"},
		{"numbers", "123-456"},
		{"colon", "source:containerd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := workloadURL(false, false, tt.search)
			require.NoError(t, err)
			assert.Contains(t, url, fmt.Sprintf("search=%s", tt.search))
		})
	}
}
