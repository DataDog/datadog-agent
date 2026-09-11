// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sameBackingArray(a, b []string) bool {
	return unsafe.SliceData(a) == unsafe.SliceData(b)
}

// realisticK8sTags mirrors a typical 20-tag Kubernetes log's tag set.
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

// noMatchFilter is configured but shares no key with realisticK8sTags.
func noMatchFilter() *Scoped {
	global, _ := Compile(nil, []string{"nonexistent_key_a:*", "nonexistent_key_b:*"})
	return NewScoped(global, nil)
}

// withDropsFilter excludes four keys present in realisticK8sTags.
func withDropsFilter() *Scoped {
	global, _ := Compile(nil, []string{"container_id:*", "kube_replica_set:*", "pod_name:*", "log_hash:*"})
	return NewScoped(global, nil)
}

func TestReportIsEmpty(t *testing.T) {
	assert.True(t, Report{}.IsEmpty())
	assert.False(t, Report{Warnings: []string{"x"}}.IsEmpty())
	assert.False(t, Report{Rejected: []RejectedPattern{{Pattern: "x", Reason: "y"}}}.IsEmpty())
}

func TestPatternsIsEmpty(t *testing.T) {
	assert.True(t, Patterns{}.IsEmpty())
	assert.False(t, Patterns{Include: []string{"x"}}.IsEmpty())
	assert.False(t, Patterns{Exclude: []string{"x"}}.IsEmpty())
}

func TestCompileNilAndEmptyInputsAreEquivalent(t *testing.T) {
	f1, r1 := Compile(nil, nil)
	f2, r2 := Compile([]string{}, []string{})
	assert.Equal(t, r1, r2)
	assert.True(t, f1.IsEmpty())
	assert.True(t, f2.IsEmpty())
}

func TestCompileRejectsInvalidPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"missing colon", "container_id"},
		{"missing colon, arbitrary text", "not-a-pattern-at-all"},
		{"bare star", "*"},
		{"star colon star", "*:*"},
		{"empty pattern", ""},
		{"whitespace-only pattern", "   "},
		{"empty key", ":foo"},
		{"wildcard in key", "cont*iner:foo"},
		{"wildcard key with star value", "foo*:*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, includeReport := Compile([]string{tt.pattern}, nil)
			requireActionableRejection(t, includeReport, tt.pattern)

			excludeFilters, excludeReport := Compile(nil, []string{tt.pattern})
			requireActionableRejection(t, excludeReport, tt.pattern)
			assert.True(t, excludeFilters.IsEmpty(), "a rejected exclude pattern must not apply")
		})
	}
}

func requireActionableRejection(t *testing.T, report Report, pattern string) {
	t.Helper()
	require.Len(t, report.Rejected, 1)
	reason := report.Rejected[0].Reason
	assert.NotEmpty(t, reason)
	trimmed := strings.TrimSpace(pattern)
	assert.True(t, strings.Contains(reason, trimmed) || strings.Contains(reason, trimmed+":*"),
		"reason %q for pattern %q should mention the pattern or the fix", reason, pattern)
}

func TestMissingColonReasonSuggestsFix(t *testing.T) {
	_, report := Compile([]string{"container_id"}, nil)
	require.Len(t, report.Rejected, 1)
	assert.Contains(t, report.Rejected[0].Reason, `"container_id:*"`)
}

func TestCompileReportsAllRejectionsInOrderAndKeepsTheRest(t *testing.T) {
	patterns := []string{"container_id", "*", "good:*", ":empty-key", "*:*"}
	f, report := Compile(nil, patterns)
	require.Len(t, report.Rejected, 4)
	assert.Equal(t, "container_id", report.Rejected[0].Pattern)
	assert.Equal(t, "*", report.Rejected[1].Pattern)
	assert.Equal(t, ":empty-key", report.Rejected[2].Pattern)
	assert.Equal(t, "*:*", report.Rejected[3].Pattern)
	assert.Equal(t, []string{"good:*"}, f.Patterns().Exclude)
}

func TestCompileDedupesEmptyAndWhitespacePatterns(t *testing.T) {
	_, report := Compile([]string{"", "   ", "\t"}, nil)
	assert.Len(t, report.Rejected, 1)
}

func TestCompileDeduplicatesPatterns(t *testing.T) {
	f, report := Compile([]string{"foo:*", " foo:* ", "foo:*"}, nil)
	assert.Empty(t, report.Rejected)
	assert.Equal(t, []string{"foo:*"}, f.Patterns().Include)
}

func TestCompileDoesNotDedupeAcrossIncludeAndExclude(t *testing.T) {
	f, report := Compile([]string{"foo:*"}, []string{"foo:*"})
	assert.Empty(t, report.Rejected)
	assert.Equal(t, []string{"foo:*"}, f.Patterns().Include)
	assert.Equal(t, []string{"foo:*"}, f.Patterns().Exclude)
	assert.True(t, f.Retains("foo:bar"), "the include rescues the identical exclude")
}

func TestCompileWarnsOnUppercaseKey(t *testing.T) {
	_, report := Compile(nil, []string{"Kube_Namespace:default"})
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0], "Kube_Namespace:default")
	assert.Contains(t, report.Warnings[0], "kube_namespace")
}

func TestCompileWarnsOnProtectedExclude(t *testing.T) {
	_, report := Compile(nil, []string{"source:foo"})
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0], "source")
	assert.Contains(t, report.Warnings[0], "no effect")
}

func TestCompileIncludeNamingProtectedKeyWarnsNothing(t *testing.T) {
	_, report := Compile([]string{"source:foo"}, nil)
	assert.Empty(t, report.Warnings)
}

func TestCompileWarnsBothUppercaseAndProtectedExclude(t *testing.T) {
	_, report := Compile(nil, []string{"Source:foo"})
	assert.Len(t, report.Warnings, 2)
}

func TestIncludeOnlyRemovesNothing(t *testing.T) {
	f, _ := Compile([]string{"foo:keep*"}, nil)
	assert.True(t, f.Retains("foo:keep1"))
	assert.True(t, f.Retains("foo:anything-else"))
	assert.True(t, f.IsEmpty())
}

func TestFiltersIsEmpty(t *testing.T) {
	empty, _ := Compile(nil, nil)
	assert.True(t, empty.IsEmpty())

	includeOnly, _ := Compile([]string{"foo:*"}, nil)
	assert.True(t, includeOnly.IsEmpty(), "include-only filters remove nothing")

	withExclude, _ := Compile(nil, []string{"foo:*"})
	assert.False(t, withExclude.IsEmpty())

	rejectedOnly, _ := Compile(nil, []string{"bad-pattern"})
	assert.True(t, rejectedOnly.IsEmpty())

	var nilFilters *Filters
	assert.True(t, nilFilters.IsEmpty())
}

func TestFiltersPatterns(t *testing.T) {
	f, report := Compile([]string{"Foo:*", " bar:baz ", "bar:baz"}, []string{"qux:*"})
	assert.Empty(t, report.Rejected)
	assert.Len(t, report.Warnings, 1)

	got := f.Patterns()
	assert.ElementsMatch(t, []string{"Foo:*", "bar:baz"}, got.Include)
	assert.Equal(t, []string{"qux:*"}, got.Exclude)

	var nilFilters *Filters
	assert.Equal(t, Patterns{}, nilFilters.Patterns())
}

func TestScopedPatternAccessors(t *testing.T) {
	global, _ := Compile(nil, []string{"g:*"})
	source, _ := Compile([]string{"s:*"}, nil)
	scoped := NewScoped(global, source)
	require.NotNil(t, scoped)

	gp := scoped.GlobalPatterns()
	assert.Empty(t, gp.Include)
	assert.Equal(t, []string{"g:*"}, gp.Exclude)

	sp := scoped.SourcePatterns()
	assert.Equal(t, []string{"s:*"}, sp.Include)
	assert.Empty(t, sp.Exclude)

	var nilScoped *Scoped
	assert.Equal(t, Patterns{}, nilScoped.GlobalPatterns())
	assert.Equal(t, Patterns{}, nilScoped.SourcePatterns())
}

func TestNilFiltersIsSafe(t *testing.T) {
	var f *Filters
	assert.True(t, f.IsEmpty())
	assert.Equal(t, Patterns{}, f.Patterns())
	assert.True(t, f.Retains("any:thing"))
	assert.True(t, f.Retains("no-colon"))

	tags := []string{"a:b", "c:d"}
	got := f.Keep(tags)
	assert.True(t, sameBackingArray(tags, got))
}

func TestNilScopedIsSafe(t *testing.T) {
	var s *Scoped
	assert.True(t, s.IsEmpty())
	assert.Equal(t, Patterns{}, s.GlobalPatterns())
	assert.Equal(t, Patterns{}, s.SourcePatterns())
	assert.True(t, s.Retains("any:thing"))
	assert.True(t, s.Retains("no-colon"))

	tags := []string{"a:b", "c:d"}
	got := s.Keep(tags)
	assert.True(t, sameBackingArray(tags, got))
}

func TestNewScopedNilNilReturnsNil(t *testing.T) {
	assert.Nil(t, NewScoped(nil, nil))
}

func TestNewScopedEmptyReturnsNil(t *testing.T) {
	empty, _ := Compile(nil, nil)
	includeOnly, _ := Compile([]string{"foo:*"}, nil)

	assert.Nil(t, NewScoped(empty, includeOnly))
	assert.Nil(t, NewScoped(nil, empty))
	assert.Nil(t, NewScoped(empty, nil))
}

func TestNewScopedNonEmptyReturnsScoped(t *testing.T) {
	withExclude, _ := Compile(nil, []string{"foo:*"})
	assert.NotNil(t, NewScoped(withExclude, nil))
	assert.NotNil(t, NewScoped(nil, withExclude))
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
			name:          "protected key retained despite matching source and global excludes",
			globalExclude: []string{"source:*"},
			sourceExclude: []string{"source:*"},
			tag:           "source:svc",
			want:          true,
		},
		{
			name:          "source include retains over source exclude on the same key",
			sourceInclude: []string{"foo:keep*"},
			sourceExclude: []string{"foo:*"},
			tag:           "foo:keep1",
			want:          true,
		},
		{
			name:          "source exclude removes when nothing rescues it",
			sourceExclude: []string{"foo:*"},
			tag:           "foo:bar",
			want:          false,
		},
		{
			name:          "global include retains when source has no opinion",
			globalInclude: []string{"foo:keep*"},
			globalExclude: []string{"foo:*"},
			tag:           "foo:keep1",
			want:          true,
		},
		{
			name:          "global exclude removes when source has no opinion",
			globalExclude: []string{"foo:*"},
			tag:           "foo:bar",
			want:          false,
		},
		{
			name:          "no rule for this key retains by default",
			globalExclude: []string{"another_key:*"},
			sourceExclude: []string{"unrelated_key:*"},
			tag:           "foo:bar",
			want:          true,
		},
		{
			name:          "source include rescues a tag global would exclude",
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
		})
	}
}

// TestScopedIsNotSequential pins the case that a sequential source-then-global
// implementation gets wrong: source has no exclude of its own for this key, so a
// sequential apply would fall through to global's exclude and drop the tag.
func TestScopedIsNotSequential(t *testing.T) {
	global, _ := Compile(nil, []string{"pod_name:*"})
	source, _ := Compile([]string{"pod_name:keep-*"}, nil)
	scoped := NewScoped(global, source)
	require.NotNil(t, scoped)

	assert.True(t, scoped.Retains("pod_name:keep-me"))
	assert.False(t, scoped.Retains("pod_name:drop-me"))

	kept := scoped.Keep([]string{"pod_name:keep-me", "pod_name:drop-me"})
	assert.Equal(t, []string{"pod_name:keep-me"}, kept)
}

func TestProtectedKeysSurviveEveryExcludeForm(t *testing.T) {
	for _, key := range protectedKeys {
		t.Run(key, func(t *testing.T) {
			tag := key + ":anything"

			literal, _ := Compile(nil, []string{key + ":anything"})
			assert.True(t, literal.Retains(tag), "literal-value exclude")

			any, _ := Compile(nil, []string{key + ":*"})
			assert.True(t, any.Retains(tag), "key:* exclude")

			glob, _ := Compile(nil, []string{key + ":any*"})
			assert.True(t, glob.Retains(tag), "glob-value exclude")

			globalScoped := NewScoped(any, nil)
			require.NotNil(t, globalScoped)
			assert.True(t, globalScoped.Retains(tag), "global-scoped exclude")

			sourceScoped := NewScoped(nil, any)
			require.NotNil(t, sourceScoped)
			assert.True(t, sourceScoped.Retains(tag), "source-scoped exclude")
		})
	}
}

func TestKeyStarNeverInspectsValue(t *testing.T) {
	f, _ := Compile(nil, []string{"container_id:*"})
	kr := f.byKey["container_id"]
	require.NotNil(t, kr)
	assert.True(t, kr.excludeAny)
	assert.Empty(t, kr.excludeVals)
	assert.False(t, f.Retains("container_id:anything-at-all-however-long"))
}

func TestValueGlobMatching(t *testing.T) {
	// want reports whether pattern "val:<pattern>" excludes tag "val:<value>".
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{"exact match, no wildcard", "bar", "bar", true},
		{"exact mismatch, no wildcard", "bar", "baz", false},
		{"leading star matches any prefix", "*bar", "foobar", true},
		{"leading star with empty prefix", "*bar", "bar", true},
		{"leading star requires the suffix", "*bar", "foobaz", false},
		{"trailing star matches any suffix", "foo*", "foobar", true},
		{"trailing star with empty suffix", "foo*", "foo", true},
		{"trailing star requires the prefix", "foo*", "barfoo", false},
		{"star only in the middle", "foo*bar", "fooXXXbar", true},
		{"star only in the middle, empty gap", "foo*bar", "foobar", true},
		{"star only in the middle, missing tail", "foo*bar", "foobarextra", false},
		{"multiple stars", "a*b*c", "aXbYc", true},
		{"multiple stars, empty gaps", "a*b*c", "abc", true},
		{"multiple stars, missing middle segment", "a*b*c", "ac", false},
		{"a*a does not match the single character a", "a*a", "a", false},
		{"a*a matches aa", "a*a", "aa", true},
		{"a*a matches aXa", "a*a", "aXa", true},
		{"wildcard pattern against an empty value", "foo*bar", "", false},
		{"exact pattern against an empty value", "bar", "", false},
		{"literal star in the value is ordinary data", "a*b", "*", false},
		{"pattern of literal stars around content matches", "*x*", "*x*", true},
		{"key-star pattern matches the literal star value too", "*", "*", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, report := Compile(nil, []string{"val:" + tt.pattern})
			require.Empty(t, report.Rejected)
			excluded := !f.Retains("val:" + tt.value)
			assert.Equal(t, tt.want, excluded)
		})
	}
}

func TestValuelessTagsAlwaysSurvive(t *testing.T) {
	// A pattern's key can textually equal a valueless tag, but a tag with no
	// colon is never matched by any pattern.
	f, _ := Compile(nil, []string{"standalone_tag:*"})
	assert.True(t, f.Retains("standalone_tag"))

	kept := f.Keep([]string{"standalone_tag", "standalone_tag:foo"})
	assert.Equal(t, []string{"standalone_tag"}, kept)
}

func TestKeepDoesNotMutateInput(t *testing.T) {
	f, _ := Compile(nil, []string{"drop:*"})

	t.Run("nothing dropped returns the same backing array", func(t *testing.T) {
		tags := []string{"keep:1", "keep:2", "keep:3"}
		original := append([]string(nil), tags...)
		got := f.Keep(tags)
		assert.True(t, sameBackingArray(tags, got))
		assert.Equal(t, original, tags)
	})

	t.Run("something dropped leaves the input untouched", func(t *testing.T) {
		tags := []string{"keep:1", "drop:1", "keep:2", "drop:2"}
		original := append([]string(nil), tags...)
		got := f.Keep(tags)
		assert.False(t, sameBackingArray(tags, got))
		assert.Equal(t, []string{"keep:1", "keep:2"}, got)
		assert.Equal(t, original, tags)
	})

	t.Run("drop at index zero", func(t *testing.T) {
		tags := []string{"drop:1", "keep:1", "keep:2"}
		original := append([]string(nil), tags...)
		got := f.Keep(tags)
		assert.False(t, sameBackingArray(tags, got))
		assert.Equal(t, []string{"keep:1", "keep:2"}, got)
		assert.Equal(t, original, tags)
	})

	t.Run("drop at the final index", func(t *testing.T) {
		tags := []string{"keep:1", "keep:2", "drop:1"}
		original := append([]string(nil), tags...)
		got := f.Keep(tags)
		assert.False(t, sameBackingArray(tags, got))
		assert.Equal(t, []string{"keep:1", "keep:2"}, got)
		assert.Equal(t, original, tags)
	})
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

func TestApplyAgreesWithRetains(t *testing.T) {
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
		"kube_replica_set:myapp-123",
		"kube_namespace:kube-system",
		"kube_namespace:default",
		"pod_name:keep-me",
		"pod_name:drop-me",
		"filename:app.log",
		"standalone_tag",
		"env:prod",
	}

	var want []string
	for _, tag := range tags {
		if scoped.Retains(tag) {
			want = append(want, tag)
		}
	}
	assert.NotEqual(t, len(tags), len(want), "fixture should exercise at least one drop")
	assert.Equal(t, want, scoped.Keep(tags))
}

func TestApplyAgreesWithRetainsOnFixtures(t *testing.T) {
	tags := realisticK8sTags()
	for _, scoped := range []*Scoped{noMatchFilter(), withDropsFilter()} {
		var want []string
		for _, tag := range tags {
			if scoped.Retains(tag) {
				want = append(want, tag)
			}
		}
		assert.Equal(t, want, scoped.Keep(tags))
	}
}

func TestKeepConcurrent(t *testing.T) {
	f, _ := Compile([]string{"pod_name:keep-*"}, []string{"container_id:*", "pod_name:*"})
	scoped := NewScoped(f, nil)
	require.NotNil(t, scoped)

	tags := []string{
		"source:myapp", "container_id:abc123", "pod_name:keep-me", "pod_name:drop-me", "env:prod",
	}

	const goroutines = 50
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				got := scoped.Keep(tags)
				assert.Len(t, got, 3)
				_ = scoped.Retains(tags[(n+j)%len(tags)])
				_ = f.Keep(tags)
			}
		}(i)
	}
	wg.Wait()
}

func TestBenchmarkFilterIsEngaged(t *testing.T) {
	tags := realisticK8sTags()

	withDrops := withDropsFilter()
	require.NotNil(t, withDrops)
	got := withDrops.Keep(tags)
	assert.Less(t, len(got), len(tags), "the with-drops benchmark fixture must actually drop something")

	noMatch := noMatchFilter()
	require.NotNil(t, noMatch)
	gotNoMatch := noMatch.Keep(tags)
	assert.True(t, sameBackingArray(tags, gotNoMatch), "the no-match benchmark fixture must drop nothing")
}

func TestKeepNoMatchAllocatesNothing(t *testing.T) {
	scoped := noMatchFilter()
	tags := realisticK8sTags()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = scoped.Keep(tags)
	})
	assert.Equal(t, float64(0), allocs)
}

func BenchmarkKeepNoMatch(b *testing.B) {
	scoped := noMatchFilter()
	tags := realisticK8sTags()

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = scoped.Keep(tags)
	}
}

func BenchmarkKeepWithDrops(b *testing.B) {
	scoped := withDropsFilter()
	tags := realisticK8sTags()

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = scoped.Keep(tags)
	}
}

func BenchmarkRetains(b *testing.B) {
	scoped := withDropsFilter()
	tags := realisticK8sTags()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scoped.Retains(tags[i%len(tags)])
	}
}

// TestRejectionReasonsOnlySuggestValidPatterns pins that any pattern a
// rejection reason recommends is itself accepted by Compile. A reason that
// suggests p+":*" for a pattern already containing "*" would put the "*" in
// the key, which Compile also rejects -- advice the user cannot act on.
func TestRejectionReasonsOnlySuggestValidPatterns(t *testing.T) {
	candidates := []string{
		"", "   ", "*", "*:*", ":foo", "foo", "image*", "test_tag_key*",
		"kube_*", "kube*:*", "*:web", "a*b", "foo:", "foo:*", "FOO:bar",
	}

	quoted := regexp.MustCompile(`"([^"]*)"`)
	for _, c := range candidates {
		_, report := Compile(nil, []string{c})
		for _, rej := range report.Rejected {
			for _, m := range quoted.FindAllStringSubmatch(rej.Reason, -1) {
				suggestion := m[1]
				// The reason echoes the offending pattern too; only check the others.
				if suggestion == rej.Pattern || suggestion == "*" {
					continue
				}
				_, sugReport := Compile(nil, []string{suggestion})
				assert.Empty(t, sugReport.Rejected,
					"pattern %q was rejected with reason %q, which suggests %q -- but %q is itself rejected",
					c, rej.Reason, suggestion, suggestion)
			}
		}
	}
}
