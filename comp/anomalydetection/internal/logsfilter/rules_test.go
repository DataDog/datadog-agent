// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewRules validation ---

func TestNewRules_RequiresName(t *testing.T) {
	_, err := NewRules([]ProcessingRule{{Type: "exclude_at_match"}})
	assert.ErrorContains(t, err, "name is required")
}

func TestNewRules_UnknownType(t *testing.T) {
	_, err := NewRules([]ProcessingRule{{Name: "r", Type: "unknown"}})
	assert.ErrorContains(t, err, "unsupported type")
}

func TestNewRules_EmptyTagRejected(t *testing.T) {
	_, err := NewRules([]ProcessingRule{{
		Name: "r", Type: "exclude_at_match", Tags: []string{"env:dev", ""},
	}})
	assert.ErrorContains(t, err, "empty values")
}

func TestNewRules_Empty(t *testing.T) {
	r, err := NewRules(nil)
	require.NoError(t, err)
	assert.True(t, r.IsAllowed("any", nil))
}

// --- IsAllowed ---

func rules(t *testing.T, rs ...ProcessingRule) *Rules {
	t.Helper()
	r, err := NewRules(rs)
	require.NoError(t, err)
	return r
}

func TestIsAllowed_NilAllowsAll(t *testing.T) {
	var r *Rules
	assert.True(t, r.IsAllowed("anything", []string{"env:dev"}))
}

func TestIsAllowed_NoRulesAllowsAll(t *testing.T) {
	r := rules(t)
	assert.True(t, r.IsAllowed("containerd", []string{"env:dev"}))
}

func TestIsAllowed_ExcludeBySourceOnly(t *testing.T) {
	r := rules(t, ProcessingRule{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"})
	assert.False(t, r.IsAllowed("containerd", nil))
	assert.True(t, r.IsAllowed("docker", nil))
}

func TestIsAllowed_ExcludeByTagOnly(t *testing.T) {
	r := rules(t, ProcessingRule{Name: "drop_dev", Type: "exclude_at_match", Tags: []string{"env:dev"}})
	assert.False(t, r.IsAllowed("containerd", []string{"env:dev", "service:foo"}))
	assert.True(t, r.IsAllowed("containerd", []string{"env:prod"}))
	assert.True(t, r.IsAllowed("containerd", nil))
}

func TestIsAllowed_ExcludeBySourceAndTag(t *testing.T) {
	r := rules(t, ProcessingRule{
		Name: "drop_dev_containerd", Type: "exclude_at_match",
		Source: "containerd", Tags: []string{"env:dev"},
	})
	assert.False(t, r.IsAllowed("containerd", []string{"env:dev"}))
	assert.True(t, r.IsAllowed("containerd", []string{"env:prod"}))
	assert.True(t, r.IsAllowed("docker", []string{"env:dev"}))
}

func TestIsAllowed_IncludeBeforeExclude(t *testing.T) {
	r := rules(t,
		ProcessingRule{Name: "keep_prod", Type: "include_at_match", Tags: []string{"env:prod"}},
		ProcessingRule{Name: "drop_all", Type: "exclude_at_match"},
	)
	assert.True(t, r.IsAllowed("x", []string{"env:prod"}))
	assert.False(t, r.IsAllowed("x", []string{"env:dev"}))
}

func TestIsAllowed_FirstMatchWins(t *testing.T) {
	r := rules(t,
		ProcessingRule{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"},
		ProcessingRule{Name: "allow_all", Type: "include_at_match"},
	)
	// First rule matches containerd, excludes.
	assert.False(t, r.IsAllowed("containerd", nil))
	// First rule doesn't match docker; second rule matches (include).
	assert.True(t, r.IsAllowed("docker", nil))
}

func TestIsAllowed_UnmatchedAllowed(t *testing.T) {
	r := rules(t, ProcessingRule{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"})
	assert.True(t, r.IsAllowed("kubelet", nil))
}

func TestIsAllowed_AllTagsMustPresent(t *testing.T) {
	r := rules(t, ProcessingRule{
		Name: "r", Type: "exclude_at_match",
		Tags: []string{"env:dev", "team:foo"},
	})
	assert.False(t, r.IsAllowed("x", []string{"env:dev", "team:foo"}))
	assert.True(t, r.IsAllowed("x", []string{"env:dev"})) // missing team:foo
}
