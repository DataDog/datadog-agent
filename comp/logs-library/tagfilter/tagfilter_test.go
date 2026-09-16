// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sameBackingArray(a, b []string) bool {
	return unsafe.SliceData(a) == unsafe.SliceData(b)
}

func realisticK8sTags() []string {
	return []string{
		"source:myapp",
		"service:myapp",
		"env:prod",
		"host:ip-10-0-1-23.ec2.internal",
		"version:1.4.2",
		"kube_namespace:default",
		"kube_pod_name:myapp-7c9f8d6b4-abcde",
		"kube_container_name:myapp",
		"kube_replica_set:myapp-7c9f8d6b4",
		"kube_deployment:myapp",
		"kube_service:myapp",
		"pod_name:myapp-7c9f8d6b4-abcde",
		"container_id:9f8d7c6b5a4321009f8d7c6b5a4321009f8d7c6b5a4321009f8d7c6b5a4321",
		"container_name:myapp",
		"image_name:myapp",
		"image_tag:1.4.2",
		"availability-zone:us-east-1a",
		"region:us-east-1",
		"instance-type:m5.large",
		"log_hash:9f8d7c6b5a432100",
	}
}

func noMatchFilter() *Scoped {
	global, _ := Compile(nil, []string{"nonexistent_key_a:*", "nonexistent_key_b:*"})
	return NewScoped(global, nil)
}

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
			_, includeReport := Compile([]string{pattern}, nil)
			require.Len(t, includeReport.Rejected, 1)
			assert.NotEmpty(t, includeReport.Rejected[0].Reason)

			excludeFilters, excludeReport := Compile(nil, []string{pattern})
			require.Len(t, excludeReport.Rejected, 1)
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

func TestCompileWarnsOnProtectedExclude(t *testing.T) {
	f, report := Compile(nil, []string{"Source:foo"})
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0], "source")
	assert.Contains(t, report.Warnings[0], "no effect")
	assert.True(t, f.IsEmpty())
	assert.Nil(t, NewScoped(f, nil))
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

func TestProtectedKeysSurviveMixedCaseTags(t *testing.T) {
	f, _ := Compile(nil, []string{"Host:*", "SERVICE:web"})
	for _, tag := range []string{"Host:myhost", "HOST:myhost", "Service:web", "sErViCe:web"} {
		assert.True(t, f.Retains(tag), "protected tag %q must survive", tag)
	}
}

func TestCaseInsensitiveLookupAllocatesNothing(t *testing.T) {
	f, _ := Compile(nil, []string{"team:in*"})
	require.False(t, f.Retains("Team:infra"))
	allocs := testing.AllocsPerRun(1000, func() {
		_ = f.Retains("Team:INFRA")
	})
	assert.Equal(t, float64(0), allocs)
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
			name:          "protected key retained",
			globalExclude: []string{"source:*", "foo:*"},
			sourceExclude: []string{"source:*"},
			tag:           "source:svc",
			want:          true,
		},
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
	assert.False(t, sameBackingArray(tags, got))
	assert.Equal(t, []string{"keep:1"}, got)
	assert.Equal(t, original, tags)

	noDrop := []string{"keep:1", "keep:2"}
	assert.True(t, sameBackingArray(noDrop, scoped.Keep(noDrop)))
}

func TestKeepAgreesWithRetains(t *testing.T) {
	global, _ := Compile(
		[]string{"kube_namespace:kube-system"},
		[]string{"container_id:*", "kube_replica_set:*", "kube_namespace:*"},
	)
	source, _ := Compile(
		[]string{"pod_name:keep-*"},
		[]string{"pod_name:*", "filename:*"},
	)
	scoped := NewScoped(global, source)
	require.NotNil(t, scoped)

	tags := []string{
		"source:myapp",
		"container_id:abc123",
		"kube_namespace:kube-system",
		"kube_namespace:default",
		"pod_name:keep-me",
		"pod_name:drop-me",
		"filename:app.log",
		"standalone_tag",
	}

	var want []string
	for _, tag := range tags {
		if scoped.Retains(tag) {
			want = append(want, tag)
		}
	}
	assert.Equal(t, want, scoped.Keep(tags))
}

func TestKeepNoMatchAllocatesNothing(t *testing.T) {
	scoped := noMatchFilter()
	tags := realisticK8sTags()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = scoped.Keep(tags)
	})
	assert.Equal(t, float64(0), allocs)
}
