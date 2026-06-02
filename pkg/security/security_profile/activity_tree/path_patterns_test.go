// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestStructureSignature(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"", ""},
		{"sess-abc", "A-A"},
		{"sess-aaa", "A-A"},
		{"sess-42", "A-N"},
		{"pod-abc123-xyz", "A-M-A"},
		{"2024-01-15.log", "N-N-N.A"},
		{"1337", "N"},
		{"config.json", "A.A"},
		{"job-001", "A-N"},
		{"job-050", "A-N"},
		{"file_v2.tar", "A_M.A"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, structureSignature(tc.name))
		})
	}
}

func TestBuildTemplate(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{"single", []string{"sess-abc"}, "sess-abc"},
		{"sess-letters", []string{"sess-aaa", "sess-bbb", "sess-ccc"}, "sess-*"},
		{"dates", []string{"2024-01-15.log", "2024-01-16.log"}, "2024-01-*.log"},
		{"jobs", []string{"job-001", "job-002", "job-050"}, "job-*"},
		{"pod-middle-only", []string{"pod-abc123-xyz", "pod-def456-xyz"}, "pod-*-xyz"},
		{"all-different", []string{"aa", "bb", "cc"}, "*"},
		{"numbers", []string{"1", "42", "1337"}, "*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildTemplate(tc.names))
		})
	}
}

func TestTemplateMatches(t *testing.T) {
	tests := []struct {
		template string
		name     string
		want     bool
	}{
		// exact match without wildcard
		{"config.json", "config.json", true},
		{"config.json", "config.yaml", false},
		// trailing wildcard
		{"sess-*", "sess-abc", true},
		{"sess-*", "sess-", false},     // wildcard must consume ≥ 1 char
		{"sess-*", "other-abc", false}, // prefix mismatch
		// leading wildcard
		{"*.log", "errors.log", true},
		{"*.log", ".log", false}, // wildcard must consume ≥ 1
		// middle wildcard
		{"pod-*-xyz", "pod-abc123-xyz", true},
		{"pod-*-xyz", "pod-xyz", false},
		{"pod-*-xyz", "pod-abc-abc", false},
		// multiple wildcards
		{"2024-01-*.log", "2024-01-15.log", true},
		{"2024-01-*.log", "2024-01-.log", false},
		// naked wildcard
		{"*", "anything", true},
		{"*", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.template+"::"+tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, templateMatches(tc.template, tc.name))
		})
	}
}

func TestGroupChildrenBySignature_SkipsPatternNodes(t *testing.T) {
	children := map[string]*FileNode{
		"sess-aaa": {Name: "sess-aaa", Children: map[string]*FileNode{}},
		"sess-bbb": {Name: "sess-bbb", Children: map[string]*FileNode{}},
		// a pre-existing pattern must not be bucketed back with literals
		"already-*": {Name: "already-*", IsPattern: true, Children: map[string]*FileNode{}},
	}
	buckets := groupChildrenBySignature(children)
	// Expect exactly one bucket for "A-A"; the pattern is excluded.
	var sigs []string
	for _, b := range buckets {
		sigs = append(sigs, b.signature)
	}
	assert.Equal(t, []string{"A-A"}, sigs)
	assert.Len(t, buckets[0].members, 2)
}

func TestMergeChildren_CollapsesSessSiblings(t *testing.T) {
	children := map[string]*FileNode{}
	for _, n := range []string{"sess-aaa", "sess-bbb", "sess-ccc"} {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 1, collapsed)
	assert.Contains(t, children, "sess-*")
	assert.NotContains(t, children, "sess-aaa")
	assert.True(t, children["sess-*"].IsPattern)
	// 3 members merged -> 2 siblings folded into 1
	assert.Equal(t, int64(2), stats.FileNodesMerged)
}

func TestMergeChildren_RespectsMinClusterSize(t *testing.T) {
	children := map[string]*FileNode{
		"sess-aaa": newTestFileLeaf("sess-aaa"),
		"sess-bbb": newTestFileLeaf("sess-bbb"),
	}
	stats := NewActivityTreeNodeStats()

	// MinClusterSize=3 → bucket of 2 is below threshold, no merge.
	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 0, collapsed)
	assert.Contains(t, children, "sess-aaa")
	assert.Contains(t, children, "sess-bbb")

	// Lower threshold to 2 → bucket now qualifies.
	collapsed = mergeChildren(children, 2, stats)
	assert.Equal(t, 1, collapsed)
	assert.Contains(t, children, "sess-*")
}

func TestMergeChildren_DoesNotMixSignatures(t *testing.T) {
	// Heterogeneous parent: numeric bucket refuses to merge to bare
	// wildcard because alpha siblings would otherwise be absorbed.
	children := map[string]*FileNode{
		"1":      newTestFileLeaf("1"),
		"42":     newTestFileLeaf("42"),
		"1337":   newTestFileLeaf("1337"),
		"99999":  newTestFileLeaf("99999"),
		"self":   newTestFileLeaf("self"),
		"stat":   newTestFileLeaf("stat"),
		"uptime": newTestFileLeaf("uptime"),
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)

	assert.Equal(t, 0, collapsed, "no bucket should merge in a heterogeneous parent")
	assert.NotContains(t, children, "*", "bare wildcard must not appear next to non-pattern siblings")
	for _, n := range []string{"1", "42", "1337", "99999", "self", "stat", "uptime"} {
		assert.Contains(t, children, n, "literal %q must survive", n)
	}
	assert.Equal(t, int64(0), stats.FileNodesMerged)
}

func TestMergeChildren_DoesNotMergeFixedAlphaTopLevelDirs(t *testing.T) {
	// Pure-alpha top-level dirs share signature "A" but must not
	// collapse into "*".
	children := map[string]*FileNode{}
	names := []string{"tmp", "var", "etc", "bin", "usr", "home"}
	for _, n := range names {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 0, collapsed)
	for _, n := range names {
		assert.Contains(t, children, n)
	}
	assert.NotContains(t, children, "*")
	assert.Equal(t, int64(0), stats.FileNodesMerged)
}

func TestIsBareWildcardTemplate(t *testing.T) {
	tests := []struct {
		template string
		want     bool
	}{
		{"", false},
		{"*", true},
		{"*-*", true},
		{"*.*", true},
		{"*_*", true},
		{"sess-*", false},
		{"*-xyz", false},
		{"pod-*-xyz", false},
		{"abc", false},
	}
	for _, tc := range tests {
		t.Run(tc.template, func(t *testing.T) {
			assert.Equal(t, tc.want, isBareWildcardTemplate(tc.template))
		})
	}
}

func TestSignatureHasVariableClass(t *testing.T) {
	tests := []struct {
		sig  string
		want bool
	}{
		{"", false},
		{"A", false},
		{"A.A", false},
		{"A-A", false},
		{"N", true},
		{"A-N", true},
		{"M", true},
		{"A-M-A", true},
		{"N-N-N.A", true},
	}
	for _, tc := range tests {
		t.Run(tc.sig, func(t *testing.T) {
			assert.Equal(t, tc.want, signatureHasVariableClass(tc.sig))
		})
	}
}

func TestFindChildWithPatternFallback(t *testing.T) {
	children := map[string]*FileNode{
		"exact.log": {Name: "exact.log"},
		"sess-*":    {Name: "sess-*", IsPattern: true},
	}
	stats := enablePatternsTestStats()

	// Exact match wins.
	c, ok := findChildWithPatternFallback(children, "exact.log", stats)
	assert.True(t, ok)
	assert.Equal(t, "exact.log", c.Name)

	// Pattern fallback.
	c, ok = findChildWithPatternFallback(children, "sess-xyz", stats)
	assert.True(t, ok)
	assert.Equal(t, "sess-*", c.Name)

	// No match.
	_, ok = findChildWithPatternFallback(children, "nope.log", stats)
	assert.False(t, ok)

	// With patterns disabled on the stats, only the exact lookup runs.
	// The sess-* pattern is ignored even though its template matches.
	disabled := NewActivityTreeNodeStats()
	_, ok = findChildWithPatternFallback(children, "sess-xyz", disabled)
	assert.False(t, ok, "disabled stats should skip the wildcard fallback")

	// Exact matches still work with patterns disabled.
	c, ok = findChildWithPatternFallback(children, "exact.log", disabled)
	assert.True(t, ok)
	assert.Equal(t, "exact.log", c.Name)
}

func TestInsertFileEvent_PatternAbsorbsVariants(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	pn.Process.FileEvent.PathnameStr = "/test/pan"
	pn.Process.Argv0 = "pan"
	stats := enablePatternsTestStats()

	count := stats.PathPatternConfig().MaxChildren + 1
	for i := 0; i < count; i++ {
		path := "/tmp/sess-" + string(rune('a'+i)) + string(rune('a'+i)) + string(rune('a'+i)) + "/file"
		event := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&event.Open.File, event, "tag", Unknown, stats, false, nil, nil)
	}

	// After insertion the /tmp node exists, and its children contain the
	// merged sess-* template rather than count literal sess-XXX nodes.
	tmp, ok := pn.Files["tmp"]
	assert.True(t, ok, "expected /tmp directory")
	assert.NotNil(t, tmp)

	var foundPattern bool
	for name, child := range tmp.Children {
		if child.IsPattern && strings.HasPrefix(name, "sess-") && strings.Contains(name, "*") {
			foundPattern = true
		}
	}
	assert.True(t, foundPattern, "expected merged sess-* pattern under /tmp, got children: %v", childrenNames(tmp.Children))
	assert.Greater(t, stats.FileNodesMerged, int64(0))
}

func TestInsertFileEvent_AnomalyDryRunQuietOnVariants(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	names := []string{"aaa", "bbb", "ccc", "ddd", "eee"}
	for _, s := range names {
		path := "/tmp/sess-" + s + "/file"
		event := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&event.Open.File, event, "tag", Unknown, stats, false, nil, nil)
	}

	// Force-merge (finalize-style) so the pattern exists even under the
	// regular threshold.
	if tmp, ok := pn.Files["tmp"]; ok {
		mergeChildren(tmp.Children, 3, stats)
	}

	newVariant := &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Open: model.OpenEvent{File: model.FileEvent{
			IsPathnameStrResolved: true,
			PathnameStr:           "/tmp/sess-zzz/file",
		}},
	}

	// Dry-run insert: no new FileNode should be created.
	before := stats.FileNodes
	isNew := pn.InsertFileEvent(&newVariant.Open.File, newVariant, "tag", Unknown, stats, true, nil, nil)
	assert.True(t, isNew == false || stats.FileNodes == before,
		"dry-run insert on pattern variant should not create new nodes")
	// The pattern-lookup-hit counter should have fired at least once.
	assert.Greater(t, stats.FilePatternLookupHits, int64(0))
}

func TestFinalizePatterns_MergesBelowMaxChildren(t *testing.T) {
	at := &ActivityTree{
		Stats: enablePatternsTestStats(),
	}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}
	stats := at.Stats

	for _, s := range []string{"aaa", "bbb", "ccc"} {
		path := "/tmp/sess-" + s + "/file"
		event := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&event.Open.File, event, "tag", Unknown, stats, false, nil, nil)
	}

	// No merge happened yet.
	tmp := pn.Files["tmp"]
	assert.NotNil(t, tmp)
	assert.Len(t, tmp.Children, 3)
	assert.Equal(t, int64(0), stats.FileNodesMerged)

	// Run finalize pass; MinClusterSizeOnFinalize defaults to 3 so the
	// bucket qualifies.
	at.FinalizePatterns()

	assert.Greater(t, stats.FileNodesMerged, int64(0))
	var hasPattern bool
	for _, child := range tmp.Children {
		if child.IsPattern {
			hasPattern = true
		}
	}
	assert.True(t, hasPattern, "finalize should have created a pattern, got: %v", childrenNames(tmp.Children))
}

func TestFinalizePatterns_PreservesFixedAlphaTopLevelDirs(t *testing.T) {
	// Regression: /tmp/*/subfolder/* must not collapse to /*/*/subfolder/*
	// when /tmp coexists with other pure-alpha top-level dirs.
	at := &ActivityTree{
		Stats: enablePatternsTestStats(),
	}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}
	stats := at.Stats

	// Enough /tmp/<pid>/subfolder/<pid> entries to exercise finalize.
	tmpPaths := []string{
		"/tmp/25739/subfolder/2008",
		"/tmp/31282/subfolder/3558",
		"/tmp/449/subfolder/9521",
		"/tmp/13860/subfolder/14273",
	}
	// Unrelated top-level dirs coexisting under the same process, all
	// with signature "A". They must not bucket-merge with tmp.
	otherPaths := []string{
		"/etc/hostname",
		"/var/log/messages",
		"/bin/sh",
		"/usr/lib/libc.so.6",
		"/proc/self/status",
	}

	for _, p := range append(append([]string{}, tmpPaths...), otherPaths...) {
		ev := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           p,
			}},
		}
		pn.InsertFileEvent(&ev.Open.File, ev, "tag", Unknown, stats, false, nil, nil)
	}

	at.FinalizePatterns()

	// Fixed alpha top-level dirs survive as literals; no bare "*" at
	// the root of the process's Files map.
	assert.NotContains(t, pn.Files, "*",
		"top-level Files should not collapse into a bare wildcard, got: %v", childrenNames(pn.Files))
	for _, d := range []string{"tmp", "etc", "var", "bin", "usr", "proc"} {
		assert.Contains(t, pn.Files, d, "top-level %q should survive as a literal", d)
	}

	// /tmp's numeric children (signature "N") are variable — they
	// collapse into a "*" pattern.
	tmp := pn.Files["tmp"]
	if assert.NotNil(t, tmp) {
		star, ok := tmp.Children["*"]
		assert.True(t, ok, "expected numeric pids under /tmp to collapse to *, got: %v", childrenNames(tmp.Children))
		if assert.NotNil(t, star) {
			sub, ok := star.Children["subfolder"]
			assert.True(t, ok, "expected /tmp/*/subfolder to survive, got: %v", childrenNames(star.Children))
			if assert.NotNil(t, sub) {
				_, ok := sub.Children["*"]
				assert.True(t, ok, "expected numeric leaves under subfolder to collapse to *, got: %v", childrenNames(sub.Children))
			}
		}
	}
}

// Heterogeneous parent with a literal-anchored bucket: the bare-wildcard
// numeric merge is refused, but prefix-* still merges.
func TestMergeChildren_HomogeneityRule_LiteralAnchorSurvivesHeterogeneity(t *testing.T) {
	children := map[string]*FileNode{
		"filename":      newTestFileLeaf("filename"),
		"412422":        newTestFileLeaf("412422"),
		"4353535":       newTestFileLeaf("4353535"),
		"63634646":      newTestFileLeaf("63634646"),
		"323526":        newTestFileLeaf("323526"),
		"prefix-32424":  newTestFileLeaf("prefix-32424"),
		"prefix-525252": newTestFileLeaf("prefix-525252"),
		"prefix-335323": newTestFileLeaf("prefix-335323"),
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 1, collapsed, "only prefix-* should merge")

	assert.NotContains(t, children, "*", "bare wildcard must not appear, got: %v", childrenNames(children))
	for _, n := range []string{"412422", "4353535", "63634646", "323526"} {
		assert.Contains(t, children, n)
	}
	assert.Contains(t, children, "filename")

	pat, ok := children["prefix-*"]
	if assert.True(t, ok) {
		assert.True(t, pat.IsPattern)
		assert.Equal(t, "A-N", pat.PatternSignature)
	}
}

// Two bare-wildcard candidate buckets with different signatures: both
// must refuse (otherwise they would collide on the "*" key).
func TestMergeChildren_HomogeneityRule_RefusesTwoBareWildcardBuckets(t *testing.T) {
	children := map[string]*FileNode{
		// signature N
		"1":     newTestFileLeaf("1"),
		"42":    newTestFileLeaf("42"),
		"1337":  newTestFileLeaf("1337"),
		"99999": newTestFileLeaf("99999"),
		// signature M
		"abc123": newTestFileLeaf("abc123"),
		"def456": newTestFileLeaf("def456"),
		"xyz789": newTestFileLeaf("xyz789"),
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)

	assert.Equal(t, 0, collapsed)
	assert.NotContains(t, children, "*")
	for _, n := range []string{"1", "42", "1337", "99999", "abc123", "def456", "xyz789"} {
		assert.Contains(t, children, n)
	}
}

// Homogeneous numeric parent: bare-wildcard merge proceeds and the
// pattern records its training signature.
func TestMergeChildren_HomogeneityRule_AllowsHomogeneousNumericFanout(t *testing.T) {
	children := map[string]*FileNode{
		"1":     newTestFileLeaf("1"),
		"42":    newTestFileLeaf("42"),
		"1337":  newTestFileLeaf("1337"),
		"99999": newTestFileLeaf("99999"),
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 1, collapsed)

	pat, ok := children["*"]
	if assert.True(t, ok) {
		assert.True(t, pat.IsPattern)
		assert.Equal(t, "N", pat.PatternSignature)
	}
	for _, n := range []string{"1", "42", "1337", "99999"} {
		assert.NotContains(t, children, n)
	}
}

// A numeric-trained "*" must not absorb a cross-class candidate at
// lookup time.
func TestFindChildWithPatternFallback_R2_RejectsCrossClass(t *testing.T) {
	children := map[string]*FileNode{
		"*": {Name: "*", IsPattern: true, PatternSignature: "N"},
	}
	stats := enablePatternsTestStats()

	c, ok := findChildWithPatternFallback(children, "424242", stats)
	if assert.True(t, ok) {
		assert.Equal(t, "*", c.Name)
	}
	_, ok = findChildWithPatternFallback(children, "malicious_binary", stats)
	assert.False(t, ok)
	_, ok = findChildWithPatternFallback(children, "abc123", stats)
	assert.False(t, ok)
}

// Same-shape candidate hits a literal-anchored pattern; different-shape
// candidate that still template-matches is refused.
func TestFindChildWithPatternFallback_R2_AcceptsCorrectShape(t *testing.T) {
	children := map[string]*FileNode{
		"prefix-*": {Name: "prefix-*", IsPattern: true, PatternSignature: "A-N"},
	}
	stats := enablePatternsTestStats()

	c, ok := findChildWithPatternFallback(children, "prefix-99", stats)
	if assert.True(t, ok) {
		assert.Equal(t, "prefix-*", c.Name)
	}
	_, ok = findChildWithPatternFallback(children, "prefix-rootkit", stats)
	assert.False(t, ok, "A-A candidate must not match an A-N-trained pattern")
}

// Patterns reloaded from proto have an empty signature and degrade to
// template-only matching.
func TestFindChildWithPatternFallback_R2_UntaggedPatternIsPermissive(t *testing.T) {
	children := map[string]*FileNode{
		"*": {Name: "*", IsPattern: true /* PatternSignature: "" */},
	}
	stats := enablePatternsTestStats()

	for _, candidate := range []string{"424242", "malicious_binary", "abc123"} {
		_, ok := findChildWithPatternFallback(children, candidate, stats)
		assert.True(t, ok, "untagged pattern must template-match %q", candidate)
	}
}

// Build a numeric-trained pattern, then dry-run an alpha leaf: the
// signature mismatch must surface the path as a new entry.
func TestInsertFileEvent_R2_AnomalyRaisedOnCrossClassVariant(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	for _, id := range []string{"1", "42", "1337", "99999"} {
		path := "/var/log/job/" + id
		ev := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&ev.Open.File, ev, "tag", Unknown, stats, false, nil, nil)
	}

	job := pn.Files["var"].Children["log"].Children["job"]
	if !assert.NotNil(t, job) {
		return
	}
	mergeChildren(job.Children, 3, stats)
	pat, ok := job.Children["*"]
	if !assert.True(t, ok) {
		return
	}
	assert.Equal(t, "N", pat.PatternSignature)

	anomaly := &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Open: model.OpenEvent{File: model.FileEvent{
			IsPathnameStrResolved: true,
			PathnameStr:           "/var/log/job/malicious_binary",
		}},
	}
	isNew := pn.InsertFileEvent(&anomaly.Open.File, anomaly, "tag", Unknown, stats, true, nil, nil)
	assert.True(t, isNew, "cross-class candidate must surface as a new entry")
}

// Dual of the previous test: a same-shape candidate hits the pattern.
func TestInsertFileEvent_R2_SameShapeStillQuiet(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	for _, id := range []string{"1", "42", "1337", "99999"} {
		path := "/var/log/job/" + id
		ev := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&ev.Open.File, ev, "tag", Unknown, stats, false, nil, nil)
	}
	job := pn.Files["var"].Children["log"].Children["job"]
	mergeChildren(job.Children, 3, stats)

	variant := &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Open: model.OpenEvent{File: model.FileEvent{
			IsPathnameStrResolved: true,
			PathnameStr:           "/var/log/job/8675309",
		}},
	}
	before := stats.FilePatternLookupHits
	isNew := pn.InsertFileEvent(&variant.Open.File, variant, "tag", Unknown, stats, true, nil, nil)
	assert.False(t, isNew, "same-shape numeric variant must match the trained pattern")
	assert.Greater(t, stats.FilePatternLookupHits, before)
}

// Pattern mining must be a no-op on trees that did not opt in (v1
// profiles, activity dumps).
func TestInsertFileEvent_DisabledByDefaultOnV1Trees(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := NewActivityTreeNodeStats()
	assert.False(t, stats.PathPatternConfig().Enabled)

	cfg := DefaultPathPatternConfig()
	count := cfg.MaxChildren + 5
	for i := 0; i < count; i++ {
		triple := string(rune('a'+i)) + string(rune('a'+i)) + string(rune('a'+i))
		path := "/tmp/sess-" + triple + "/file"
		ev := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
			Open: model.OpenEvent{File: model.FileEvent{
				IsPathnameStrResolved: true,
				PathnameStr:           path,
			}},
		}
		pn.InsertFileEvent(&ev.Open.File, ev, "tag", Unknown, stats, false, nil, nil)
	}

	tmp, ok := pn.Files["tmp"]
	if !assert.True(t, ok) {
		return
	}
	assert.Len(t, tmp.Children, count)
	for _, child := range tmp.Children {
		assert.False(t, child.IsPattern)
	}
	assert.Equal(t, int64(0), stats.FileNodesMerged)
	assert.Equal(t, int64(0), stats.FilePatternLookupHits)

	at := &ActivityTree{
		Stats:        stats,
		ProcessNodes: []*ProcessNode{pn},
	}
	at.FinalizePatterns()
	assert.Len(t, tmp.Children, count)
	assert.Equal(t, int64(0), stats.FileNodesMerged)
}

func enablePatternsTestStats() *Stats {
	stats := NewActivityTreeNodeStats()
	stats.SetPathPatternConfig(DefaultPathPatternConfig())
	return stats
}

func newTestFileLeaf(name string) *FileNode {
	fn := &FileNode{
		Name:     name,
		Children: map[string]*FileNode{},
	}
	fn.NodeBase = NewNodeBase()
	return fn
}

func childrenNames(children map[string]*FileNode) []string {
	out := make([]string, 0, len(children))
	for name := range children {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
