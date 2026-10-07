// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileRejectsInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{
		"container_id",
		"*",
		"*:*",
		"",
		"   ",
		":foo",
		"cont*iner:foo",
	} {
		t.Run(pattern, func(t *testing.T) {
			excludeFilters, excludeReport := Compile(nil, []string{pattern})
			require.Len(t, excludeReport.Rejected, 1)
			assert.NotEmpty(t, excludeReport.Rejected[0].Reason)
			assert.True(t, excludeFilters.IsEmpty())
		})
	}
}

func TestCompileReportsAllRejectionsAndKeepsValidPatterns(t *testing.T) {
	f, report := Compile(nil, []string{"container_id", "*", "good:*", ":empty-key", "*:*"})
	require.Len(t, report.Rejected, 4)
	assert.Equal(t, []string{"container_id", "*", ":empty-key", "*:*"}, []string{
		report.Rejected[0].Pattern,
		report.Rejected[1].Pattern,
		report.Rejected[2].Pattern,
		report.Rejected[3].Pattern,
	})
	assert.Equal(t, []string{"good:*"}, f.Patterns().Exclude)
}

func TestCompileDeduplicatesPatterns(t *testing.T) {
	f, report := Compile([]string{"Team:Infra", " team:infra ", "TEAM:INFRA", "Team:Infra"}, nil)
	assert.Empty(t, report.Rejected)
	assert.Equal(t, []string{"Team:Infra"}, f.Patterns().Include)
}

func TestPayloadAttributeWarningsDoNotProtectTags(t *testing.T) {
	f, report := Compile(nil, []string{"Source:*", "service:*", "HOST:*", "hostname:*", "env:*", "version:*"})
	require.Len(t, report.Warnings, 3)
	assert.Contains(t, report.Warnings[0], "source")
	assert.Contains(t, report.Warnings[1], "service")
	assert.Contains(t, report.Warnings[2], "host")
	for _, tag := range []string{"source:app", "service:web", "host:node", "hostname:node", "env:prod", "version:1"} {
		assert.False(t, f.Retains(tag))
	}
}

func TestMatchingUsesASCIICaseFolding(t *testing.T) {
	tests := []struct {
		name    string
		exclude string
		tag     string
	}{
		{"upper pattern key", "Team:*", "team:infra"},
		{"upper tag key", "team:*", "Team:infra"},
		{"upper pattern value", "team:INFRA", "team:infra"},
		{"upper tag value", "team:infra", "team:INFRA"},
		{"glob spans case", "image_tag:V1.*", "image_tag:v1.4.2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, report := Compile(nil, []string{test.exclude})
			require.Empty(t, report.Rejected)
			assert.False(t, f.Retains(test.tag))
		})
	}

	f, report := Compile(nil, []string{"city:MÜNCHEN"})
	require.Empty(t, report.Rejected)
	assert.False(t, f.Retains("city:MÜNCHEN"))
	assert.True(t, f.Retains("city:münchen"), "non-ASCII casing must be compared exactly")
}

func TestIncludeOnlyRemovesNothing(t *testing.T) {
	f, _ := Compile([]string{"foo:keep*"}, nil)
	assert.True(t, f.Retains("foo:keep1"))
	assert.True(t, f.Retains("foo:anything-else"))
	assert.True(t, f.IsEmpty())
}

func mustScoped(t *testing.T, globalInclude, globalExclude, sourceInclude, sourceExclude []string) *Scoped {
	t.Helper()
	global, _ := Compile(globalInclude, globalExclude)
	source, _ := Compile(sourceInclude, sourceExclude)
	scoped := NewScoped(global, source)
	require.NotNil(t, scoped)
	return scoped
}

func TestScopedPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		globalInclude []string
		globalExclude []string
		sourceInclude []string
		sourceExclude []string
		tag           string
		want          bool
	}{
		{
			name:          "source include wins over source exclude",
			sourceInclude: []string{"foo:keep*"},
			sourceExclude: []string{"foo:*"},
			tag:           "foo:keep1",
			want:          true,
		},
		{
			name:          "source exclude removes",
			sourceExclude: []string{"foo:*"},
			tag:           "foo:bar",
			want:          false,
		},
		{
			name:          "global include wins over global exclude",
			globalInclude: []string{"foo:keep*"},
			globalExclude: []string{"foo:*"},
			tag:           "foo:keep1",
			want:          true,
		},
		{
			name:          "global exclude removes",
			globalExclude: []string{"foo:*"},
			tag:           "foo:bar",
			want:          false,
		},
		{
			name:          "source include rescues global exclude",
			sourceInclude: []string{"foo:keep*"},
			globalExclude: []string{"foo:*"},
			tag:           "foo:keep1",
			want:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scoped := mustScoped(t, tt.globalInclude, tt.globalExclude, tt.sourceInclude, tt.sourceExclude)
			assert.Equal(t, tt.want, scoped.Retains(tt.tag))
			key, value, _ := strings.Cut(tt.tag, ":")
			assert.Equal(t, tt.want, scoped.RetainsTag(key, value))
		})
	}
}

func TestValueGlobMatching(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{"exact match", "bar", "bar", true},
		{"exact mismatch", "bar", "baz", false},
		{"leading star", "*bar", "foobar", true},
		{"trailing star", "foo*", "foobar", true},
		{"middle star", "foo*bar", "fooXXXbar", true},
		{"middle star missing tail", "foo*bar", "foobarextra", false},
		{"multiple stars", "a*b*c", "aXbYc", true},
		{"multiple stars missing segment", "a*b*c", "ac", false},
		{"repeated literal needs two characters", "a*a", "a", false},
		{"empty value", "foo*bar", "", false},
		{"key-star", "*", "anything", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, report := Compile(nil, []string{"val:" + tt.pattern})
			require.Empty(t, report.Rejected)
			assert.Equal(t, tt.want, !f.Retains("val:"+tt.value))
		})
	}
}

func TestValuelessTagsAlwaysSurvive(t *testing.T) {
	f, _ := Compile(nil, []string{"standalone_tag:*"})
	assert.True(t, f.Retains("standalone_tag"))
	assert.Equal(t, []string{"standalone_tag"}, f.Keep([]string{"standalone_tag", "standalone_tag:foo"}))
}

func TestScopedKeepDoesNotMutateInput(t *testing.T) {
	global, _ := Compile(nil, []string{"drop:*"})
	scoped := NewScoped(global, nil)
	require.NotNil(t, scoped)

	tags := []string{"keep:1", "drop:1"}
	original := append([]string(nil), tags...)
	got := scoped.Keep(tags)
	assert.Equal(t, []string{"keep:1"}, got)
	assert.Equal(t, original, tags)
}
