// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"sort"
	"strconv"
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
		{"sess-42", "A-N"},
		{"sess42", "M"},
		{"pod-abc123-xyz", "A-M-A"},
		{"2024-01-15.log", "D.A"},
		{"app-20240115.log", "A-D.A"},
		{"2024-13-15.log", "N-N-N.A"},
		{"12345678", "N"},
		{"1337", "N"},
		{"config.json", "A.A"},
		{"file_v2.tar", "A_M.A"},
		{"deadbeef", "H"},
		{"3fa9c2e1", "H"},
		{"cafe", "A"},
		{"x9y8z7", "M"},
		{"123e4567-e89b-12d3-a456-426614174000", "U"},
		{"123e4567-e89b-12d3-a456-426614174000.json", "U.A"},
		{"a@b", "\x00a@b\x00"},
		{"*", "\x00*\x00"},
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
		{"sess-letters", []string{"sess-aaa", "sess-bbb", "sess-ccc"}, "sess-<alpha>"},
		{"sess-numbers", []string{"sess-1", "sess-22"}, "sess-<num>"},
		{"no-separator-prefix", []string{"sess1", "sess22", "sess333"}, "sess<num>"},
		{"prefix-not-cut-mid-number", []string{"sess42", "sess43"}, "sess<num>"},
		{"no-separator-suffix", []string{"2024backup", "2025backup"}, "<num>backup"},
		{"prefix-mixed-middle", []string{"pod7abc", "pod8xyz"}, "pod<alnum>"},
		{"random-ids", []string{"a7k2x9", "q9w8e7", "z3x8c1"}, "<alnum>"},
		{"dates", []string{"2024-01-15.log", "2024-01-16.log"}, "<date>.log"},
		{"timestamps", []string{"app-2024-01-15T10-30-00.log", "app-20240201-093000.log"}, "app-<date>.log"},
		{"pod-middle-only", []string{"pod-abc123-xyz", "pod-def456-xyz"}, "pod-<alnum>-xyz"},
		{"numbers", []string{"1", "42", "1337"}, "<num>"},
		{"hex", []string{"deadbeef", "3fa9c2e1"}, "<hex>"},
		{"uuids", []string{"123e4567-e89b-12d3-a456-426614174000", "00000000-0000-0000-0000-000000000000"}, "<uuid>"},
		{"alpha-only", []string{"aa", "bb", "cc"}, "<alpha>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildTemplate(tc.names))
		})
	}
}

func TestPatternMatches(t *testing.T) {
	tests := []struct {
		template string
		name     string
		want     bool
	}{
		{"sess-<alpha>", "sess-abc", true},
		{"sess-<alpha>", "sess-", false},
		{"sess-<alpha>", "sess-42", false},
		{"sess-<alpha>", "other-abc", false},
		{"sess<num>", "sess42", true},
		{"sess<num>", "session42", false},
		{"<num>.log", "42.log", true},
		{"<num>.log", "errors.log", false},
		{"pod-<alnum>-xyz", "pod-abc123-xyz", true},
		{"pod-<alnum>-xyz", "pod-abc-xyz", false},
		// <alnum> needs both a letter and a digit
		{"<alnum>", "a7k2", true},
		{"<alnum>", "7a", true},
		{"<alnum>", "backdoor", false},
		{"<alnum>", "123", false},
		{"2024-01-<num>.log", "2024-01-15.log", true},
		{"2024-01-<num>.log", "2024-01-.log", false},
		{"<num>", "8675309", true},
		{"<num>", "self", false},
		{"<num>", "", false},
		{"<hex>", "deadbeef", true},
		{"<hex>", "cafe", false},
		{"<uuid>", "123e4567-e89b-12d3-a456-426614174000", true},
		{"<uuid>", "123e4567", false},
		{"app-<date>.log", "app-2025-12-31.log", true},
		{"app-<date>.log", "app-20251231.log", true},
		{"app-<date>.log", "app-2025-12-31T23-59-59Z.log", true},
		{"app-<date>.log", "app-2025-13-31.log", false},
		{"app-<date>.log", "app-12345678.log", false},
		{"app-<date>.log", "app-evil.log", false},
		// literal regex metacharacters are escaped
		{"a.b-<num>", "a.b-1", true},
		{"a.b-<num>", "axb-1", false},
	}
	for _, tc := range tests {
		t.Run(tc.template+"::"+tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, compilePattern(tc.template).re.MatchString(tc.name))
		})
	}
}

func TestIsPatternName(t *testing.T) {
	assert.True(t, isPatternName("<num>"))
	assert.True(t, isPatternName("sess-<alpha>"))
	// PathsReducer output is a literal, not a mined pattern
	assert.False(t, isPatternName("*"))
	assert.False(t, isPatternName("kubepods-*.slice"))
	assert.False(t, isPatternName("<unknown>"))
	assert.False(t, isPatternName("a<b"))
}

func TestTemplateScopeOf(t *testing.T) {
	tests := []struct {
		template string
		want     templateScope
	}{
		{"", scopeNone},
		{"plain", scopeNone},
		{"<alpha>", scopeNone},
		{"<alpha>-<alpha>", scopeNone},
		{"<num>", scopeAll},
		{"<hex>", scopeAll},
		{"<alnum>", scopeDirectories},
		{"<alnum>-<alnum>", scopeDirectories},
		{"<alpha>-<alnum>", scopeDirectories},
		{"<alnum>-<num>", scopeAll},
		{"tmp.<alnum>", scopeAll},
		{"sess-<alpha>", scopeAll},
		{"<alpha>.log", scopeAll},
	}
	for _, tc := range tests {
		t.Run(tc.template, func(t *testing.T) {
			assert.Equal(t, tc.want, templateScopeOf(tc.template))
		})
	}
}

func TestGroupChildrenBySignature_SkipsPatternNodes(t *testing.T) {
	children := map[string]*FileNode{
		"sess-aaa": {Name: "sess-aaa", Children: map[string]*FileNode{}},
		"sess-bbb": {Name: "sess-bbb", Children: map[string]*FileNode{}},
		// a pre-existing pattern must not be bucketed back with literals
		"already-<alpha>": {Name: "already-<alpha>", IsPattern: true, Children: map[string]*FileNode{}},
	}
	buckets := groupChildrenBySignature(children)
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
	assert.Contains(t, children, "sess-<alpha>")
	assert.NotContains(t, children, "sess-aaa")
	assert.True(t, children["sess-<alpha>"].IsPattern)
	// 3 members merged -> 2 siblings folded into 1
	assert.Equal(t, int64(2), stats.FileNodesMerged)
}

func TestMergeChildren_RespectsMinGroupSize(t *testing.T) {
	children := map[string]*FileNode{
		"sess-aaa": newTestFileLeaf("sess-aaa"),
		"sess-bbb": newTestFileLeaf("sess-bbb"),
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)
	assert.Equal(t, 0, collapsed)
	assert.Contains(t, children, "sess-aaa")
	assert.Contains(t, children, "sess-bbb")

	collapsed = mergeChildren(children, 2, stats)
	assert.Equal(t, 1, collapsed)
	assert.Contains(t, children, "sess-<alpha>")
}

// A typed numeric pattern can coexist with fixed names: it cannot absorb
// them, so no homogeneity requirement applies.
func TestMergeChildren_NumericCollapsesNextToFixedNames(t *testing.T) {
	children := map[string]*FileNode{}
	for _, n := range []string{"1", "42", "1337", "99999", "self", "stat", "uptime"} {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)

	assert.Equal(t, 1, collapsed)
	assert.Contains(t, children, "<num>")
	for _, n := range []string{"self", "stat", "uptime"} {
		assert.Contains(t, children, n, "literal %q must survive", n)
	}
	assert.Equal(t, int64(3), stats.FileNodesMerged)
}

func TestMergeChildren_DoesNotMergeFixedAlphaTopLevelDirs(t *testing.T) {
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
	assert.NotContains(t, children, "<alpha>")
	assert.Equal(t, int64(0), stats.FileNodesMerged)
}

// Differently-shaped buckets produce distinct typed templates.
func TestMergeChildren_TypedBucketsCoexist(t *testing.T) {
	children := map[string]*FileNode{}
	for _, n := range []string{
		"1", "42", "1337", // <num>
		"prefix-32424", "prefix-525252", "prefix-335323", // prefix-<num>
		"tmp.5nVjCHLMxA", "tmp.Qx7bK2pLmZ", "tmp.a9Fk3JdW0e", // tmp.<alnum>
		"filename",
	} {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)

	assert.Equal(t, 3, collapsed)
	assert.Equal(t, []string{"<num>", "filename", "prefix-<num>", "tmp.<alnum>"}, childrenNames(children))
}

// A word+number family and unrelated random IDs share a signature; they
// must produce separate patterns instead of one loose <alnum>.
func TestMergeChildren_SplitsMixedFamiliesByAffix(t *testing.T) {
	newChildren := func() map[string]*FileNode {
		children := map[string]*FileNode{}
		for _, n := range []string{
			"job.sess1", "job.sess2", "job.sess3", "job.sess4", "job.sess5",
			"job.5nVjCHLMxA", "job.Qx7bK2pLmZ", "job.a9Fk3JdW0e",
		} {
			children[n] = newTestFileLeaf(n)
		}
		return children
	}

	// finalize-style threshold: both families qualify
	children := newChildren()
	mergeChildren(children, 3, NewActivityTreeNodeStats())
	assert.Equal(t, []string{"job.<alnum>", "job.sess<num>"}, childrenNames(children))

	// insert-time threshold: the 3 random IDs are not enough evidence
	children = newChildren()
	mergeChildren(children, 5, NewActivityTreeNodeStats())
	assert.Equal(t, []string{"job.5nVjCHLMxA", "job.Qx7bK2pLmZ", "job.a9Fk3JdW0e", "job.sess<num>"}, childrenNames(children))

	stats := enablePatternsTestStats()
	children = newChildren()
	mergeChildren(children, 3, stats)
	for name, want := range map[string]string{
		"job.sess99": "job.sess<num>",
		"job.evil42": "job.<alnum>",
	} {
		c, ok := findChildWithPatternFallback(children, name, true, stats)
		if assert.True(t, ok, name) {
			assert.Equal(t, want, c.Name, name)
		}
	}
	_, ok := findChildWithPatternFallback(children, "job.backdoor", true, stats)
	assert.False(t, ok, "pure-alpha names must not match <alnum>")
}

func TestLooksRandom(t *testing.T) {
	for name, want := range map[string]bool{
		"5nVjCHLMxA":   true,
		"4fo3s6f9":     true,
		"tmp.a9Fk3JdW": true,
		"utf8":         false,
		"gb2312":       false,
		"urllib3":      false,
		"EST5EDT":      false,
		"34JTBSJW":     false,
		"a7k2":         false,
		"session":      false,
	} {
		assert.Equal(t, want, looksRandom(name), name)
	}
}

// Real fixed names with letters and digits (charsets, timezones, Python
// packages) must stay literal: they share no affix and do not look random.
// The random leaves would only yield a bare <alnum>, reserved to directories.
func TestMergeChildren_FixedAlnumNamesStayLiteral(t *testing.T) {
	for _, names := range [][]string{
		{"utf8", "big5", "gb2312", "koi8r", "latin1"},
		{"EST5EDT", "PST8PDT", "CST6CDT", "MST7MDT"},
		{"jinja2", "urllib3", "bs4", "boto3"},
		{"a7k2x9", "q9w8e7", "z3x8c1"},
	} {
		children := map[string]*FileNode{}
		for _, n := range names {
			children[n] = newTestFileLeaf(n)
		}

		collapsed := mergeChildren(children, 3, NewActivityTreeNodeStats())

		assert.Equal(t, 0, collapsed, "%v", names)
		assert.Len(t, children, len(names), "%v", names)
	}
}

// Leftover names merge only when most of them look generated.
func TestMergeChildren_PoolRequiresRandomLookingNames(t *testing.T) {
	children := map[string]*FileNode{}
	for _, n := range []string{"tmp.utf8", "tmp.big5", "tmp.gb2312", "tmp.5nVjCHLMxA"} {
		children[n] = newTestFileLeaf(n)
	}
	assert.Equal(t, 0, mergeChildren(children, 3, NewActivityTreeNodeStats()))

	children = map[string]*FileNode{}
	for _, n := range []string{"dda-telemetry-4fo3s6f9", "dda-telemetry-x1y2z3w4", "dda-telemetry-k8s9m0q1", "dda-telemetry-utf8"} {
		children[n] = newTestFileLeaf(n)
	}
	assert.Equal(t, 1, mergeChildren(children, 3, NewActivityTreeNodeStats()))
	assert.Equal(t, []string{"dda-telemetry-<alnum>"}, childrenNames(children))
}

// Random directory names with no fixed part collapse into a bare <alnum>
// that only matches path components followed by more of the path.
func TestMergeChildren_BareAlnumOnlyForDirectories(t *testing.T) {
	children := map[string]*FileNode{}
	for _, n := range []string{"5nVjCHLMxA", "Qx7bK2pLmZ", "a9Fk3JdW0e"} {
		dir := newTestFileLeaf(n)
		dir.Children["T"] = newTestFileLeaf("T")
		children[n] = dir
	}
	children["latin1"] = newTestFileLeaf("latin1")
	stats := enablePatternsTestStats()

	assert.Equal(t, 1, mergeChildren(children, 3, stats))
	assert.Equal(t, []string{"<alnum>", "latin1"}, childrenNames(children))

	c, ok := findChildWithPatternFallback(children, "Zq8wX7vB6n", false, stats)
	if assert.True(t, ok) {
		assert.Equal(t, "<alnum>", c.Name)
	}
	_, ok = findChildWithPatternFallback(children, "Zq8wX7vB6n", true, stats)
	assert.False(t, ok, "a directory-only pattern must not match a file")
}

func TestDateLenAt(t *testing.T) {
	for name, want := range map[string]int{
		"2024-01-15":          10,
		"2024_01_15.log":      10,
		"20240115":            8,
		"2024-01-15T10-30-00": 19,
		"20240115T103000Z":    16,
		"2024-01-15_10:30:00": 19,
		"2024-01-15-1.log":    10,
		"2024-01-15abc":       0,
		"2024-01-15T10-3000":  0,
		"2024-13-01":          0,
		"2024-01-32":          0,
		"2024-01_15":          0,
		"1824-01-15":          0,
		"12345678":            0,
		"202401151":           0,
	} {
		assert.Equal(t, want, dateLenAt(name, 0), name)
	}
}

// Two dated siblings are enough when the date is what varies, and the
// resulting template keeps matching later months and years.
func TestMergeChildren_DatesNeedOnlyTwoMembers(t *testing.T) {
	stats := enablePatternsTestStats()
	children := map[string]*FileNode{}
	for _, n := range []string{"app-2024-01-15.log", "app-2024-01-16.log", "app.log"} {
		children[n] = newTestFileLeaf(n)
	}

	assert.Equal(t, 1, mergeChildren(children, 3, stats))
	assert.Equal(t, []string{"app-<date>.log", "app.log"}, childrenNames(children))
	c, ok := findChildWithPatternFallback(children, "app-2025-03-01.log", true, stats)
	if assert.True(t, ok) {
		assert.Equal(t, "app-<date>.log", c.Name)
	}
}

// The lower threshold only applies when the date varies.
func TestMergeChildren_LowDateThresholdNeedsVaryingDate(t *testing.T) {
	stats := enablePatternsTestStats()
	for _, names := range [][]string{
		{"2024-01-15-a.log", "2024-01-15-b.log"},
		{"sess-1", "sess-2"},
	} {
		children := map[string]*FileNode{}
		for _, n := range names {
			children[n] = newTestFileLeaf(n)
		}
		assert.Equal(t, 0, mergeChildren(children, 3, stats), "%v", names)
	}
}

func TestInsertFileEvent_DatedLogsQuietNextMonth(t *testing.T) {
	at := &ActivityTree{Stats: enablePatternsTestStats()}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}
	for _, p := range []string{"/var/log/app/app-2024-01-15.log", "/var/log/app/app-2024-01-16.log"} {
		insertTestPath(pn, p, at.Stats, false)
	}

	at.FinalizePatterns()

	app := pn.Files["var"].Children["log"].Children["app"]
	if !assert.Equal(t, []string{"app-<date>.log"}, childrenNames(app.Children)) {
		return
	}
	assert.False(t, insertTestPath(pn, "/var/log/app/app-2024-02-01.log", at.Stats, true))
	assert.True(t, insertTestPath(pn, "/var/log/app/app-evil.log", at.Stats, true))
}

func TestInsertFileEvent_BareAlnumDirectoryKeepsCheckingBelow(t *testing.T) {
	at := &ActivityTree{Stats: enablePatternsTestStats()}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}
	for _, id := range []string{"5nVjCHLMxA", "Qx7bK2pLmZ", "a9Fk3JdW0e"} {
		insertTestPath(pn, "/var/folders/"+id+"/T/cache", at.Stats, false)
	}

	at.FinalizePatterns()

	folders := pn.Files["var"].Children["folders"]
	if !assert.Equal(t, []string{"<alnum>"}, childrenNames(folders.Children)) {
		return
	}
	assert.False(t, insertTestPath(pn, "/var/folders/Zq8wX7vB6n/T/cache", at.Stats, true))
	assert.True(t, insertTestPath(pn, "/var/folders/Zq8wX7vB6n", at.Stats, true), "a file must not match a directory-only pattern")
	assert.True(t, insertTestPath(pn, "/var/folders/Zq8wX7vB6n/T/evil", at.Stats, true), "paths below the pattern are still checked")
}

func TestMergeChildren_HexHashesGroupTogether(t *testing.T) {
	children := map[string]*FileNode{}
	// "deadbeef" has no digit but still groups with the other hashes
	for _, n := range []string{"deadbeef", "3fa9c2e1", "0a1b2c3d4e5f", "cafe"} {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	collapsed := mergeChildren(children, 3, stats)

	assert.Equal(t, 1, collapsed)
	assert.Equal(t, []string{"<hex>", "cafe"}, childrenNames(children))
}

func TestMergeChildren_ExistingPatternAbsorbsNewBucket(t *testing.T) {
	children := map[string]*FileNode{
		"<num>": {Name: "<num>", IsPattern: true, Children: map[string]*FileNode{}, NodeBase: NewNodeBase()},
	}
	for _, n := range []string{"7", "8", "9"} {
		children[n] = newTestFileLeaf(n)
	}
	stats := NewActivityTreeNodeStats()

	mergeChildren(children, 3, stats)

	assert.Equal(t, []string{"<num>"}, childrenNames(children))
}

func TestCollapseBucket_RewritesPathnames(t *testing.T) {
	children := map[string]*FileNode{}
	for _, id := range []string{"1001", "1002", "1003"} {
		dir := NewFileNode(nil, nil, id, 0, Unknown, "", nil)
		dir.Children["out.log"] = NewFileNode(&model.FileEvent{}, nil, "out.log", 0, Unknown, "/var/job/"+id+"/out.log", nil)
		children[id] = dir
	}
	children["1004"] = NewFileNode(&model.FileEvent{}, nil, "1004", 0, Unknown, "/var/job/1004", nil)

	mergeChildren(children, 3, NewActivityTreeNodeStats())

	pat, ok := children["<num>"]
	if !assert.True(t, ok, "got: %v", childrenNames(children)) {
		return
	}
	leaf, ok := pat.Children["out.log"]
	if assert.True(t, ok) && assert.NotNil(t, leaf.File) {
		assert.Equal(t, "/var/job/<num>/out.log", leaf.File.PathnameStr)
	}
}

func TestRulePathFromProfilePath(t *testing.T) {
	assert.Equal(t, `~"/var/job/*/out.log"`, rulePathFromProfilePath("/var/job/<num>/out.log"))
	assert.Equal(t, `~"/tmp/sess-*"`, rulePathFromProfilePath("/tmp/sess-<alpha>"))
	assert.Equal(t, `"/etc/passwd"`, rulePathFromProfilePath("/etc/passwd"))
}

func TestFindChildWithPatternFallback(t *testing.T) {
	children := map[string]*FileNode{
		"exact.log":    {Name: "exact.log"},
		"sess-<alpha>": {Name: "sess-<alpha>", IsPattern: true},
	}
	stats := enablePatternsTestStats()

	c, ok := findChildWithPatternFallback(children, "exact.log", true, stats)
	assert.True(t, ok)
	assert.Equal(t, "exact.log", c.Name)

	c, ok = findChildWithPatternFallback(children, "sess-xyz", true, stats)
	assert.True(t, ok)
	assert.Equal(t, "sess-<alpha>", c.Name)

	_, ok = findChildWithPatternFallback(children, "nope.log", true, stats)
	assert.False(t, ok)

	// With patterns disabled on the stats, only the exact lookup runs.
	disabled := NewActivityTreeNodeStats()
	_, ok = findChildWithPatternFallback(children, "sess-xyz", true, disabled)
	assert.False(t, ok, "disabled stats should skip the pattern fallback")

	c, ok = findChildWithPatternFallback(children, "exact.log", true, disabled)
	assert.True(t, ok)
	assert.Equal(t, "exact.log", c.Name)
}

func TestFindChildWithPatternFallback_RejectsCrossClass(t *testing.T) {
	children := map[string]*FileNode{
		"<num>": {Name: "<num>", IsPattern: true},
	}
	stats := enablePatternsTestStats()

	c, ok := findChildWithPatternFallback(children, "424242", true, stats)
	if assert.True(t, ok) {
		assert.Equal(t, "<num>", c.Name)
	}
	_, ok = findChildWithPatternFallback(children, "malicious_binary", true, stats)
	assert.False(t, ok)
	_, ok = findChildWithPatternFallback(children, "abc123", true, stats)
	assert.False(t, ok)
}

func TestFindChildWithPatternFallback_AcceptsCorrectShape(t *testing.T) {
	children := map[string]*FileNode{
		"prefix-<num>": {Name: "prefix-<num>", IsPattern: true},
	}
	stats := enablePatternsTestStats()

	c, ok := findChildWithPatternFallback(children, "prefix-99", true, stats)
	if assert.True(t, ok) {
		assert.Equal(t, "prefix-<num>", c.Name)
	}
	_, ok = findChildWithPatternFallback(children, "prefix-rootkit", true, stats)
	assert.False(t, ok)
}

func TestFindChildWithPatternFallback_PrefersMostSpecific(t *testing.T) {
	children := map[string]*FileNode{
		"<num>":       {Name: "<num>", IsPattern: true},
		"<hex>":       {Name: "<hex>", IsPattern: true},
		"job-<alnum>": {Name: "job-<alnum>", IsPattern: true},
		"job-<num>":   {Name: "job-<num>", IsPattern: true},
	}
	stats := enablePatternsTestStats()

	for name, want := range map[string]string{
		"12345678":  "<num>",
		"deadbeef1": "<hex>",
		"job-42":    "job-<num>",
		"job-x9y8z": "job-<alnum>",
	} {
		c, ok := findChildWithPatternFallback(children, name, true, stats)
		if assert.True(t, ok, name) {
			assert.Equal(t, want, c.Name, name)
		}
	}
}

// The class lives in the name, so a pattern decoded from a stored profile
// keeps its strictness.
func TestFindChildWithPatternFallback_StrictAfterReload(t *testing.T) {
	reloaded := &FileNode{Name: "<num>", IsPattern: isPatternName("<num>")}
	children := map[string]*FileNode{"<num>": reloaded}
	stats := enablePatternsTestStats()

	_, ok := findChildWithPatternFallback(children, "424242", true, stats)
	assert.True(t, ok)
	for _, candidate := range []string{"malicious_binary", "abc123", "etc"} {
		_, ok := findChildWithPatternFallback(children, candidate, true, stats)
		assert.False(t, ok, "reloaded <num> must not match %q", candidate)
	}
}

// A literal "*" written by the PathsReducer must not act as a wildcard.
func TestFindChildWithPatternFallback_ReducerStarIsLiteral(t *testing.T) {
	star := NewFileNode(nil, nil, "*", 0, Unknown, "", nil)
	assert.False(t, star.IsPattern)
	children := map[string]*FileNode{"*": star}
	stats := enablePatternsTestStats()

	_, ok := findChildWithPatternFallback(children, "net", true, stats)
	assert.False(t, ok)
	c, ok := findChildWithPatternFallback(children, "*", true, stats)
	assert.True(t, ok)
	assert.Same(t, star, c)
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
		triple := string(rune('a'+i)) + string(rune('a'+i)) + string(rune('a'+i))
		insertTestPath(pn, "/tmp/sess-"+triple+"/file", stats, false)
	}

	tmp, ok := pn.Files["tmp"]
	if !assert.True(t, ok, "expected /tmp directory") {
		return
	}
	assert.Contains(t, tmp.Children, "sess-<alpha>", "got: %v", childrenNames(tmp.Children))
	assert.Greater(t, stats.FileNodesMerged, int64(0))
}

func TestInsertFileEvent_AnomalyDryRunQuietOnVariants(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	for _, s := range []string{"aaa", "bbb", "ccc", "ddd", "eee"} {
		insertTestPath(pn, "/tmp/sess-"+s+"/file", stats, false)
	}
	mergeChildren(pn.Files["tmp"].Children, 3, stats)

	before := stats.FileNodes
	isNew := insertTestPath(pn, "/tmp/sess-zzz/file", stats, true)
	assert.False(t, isNew)
	assert.Equal(t, before, stats.FileNodes)
	assert.Greater(t, stats.FilePatternLookupHits, int64(0))
}

func TestFinalizePatterns_MergesBelowMaxChildren(t *testing.T) {
	at := &ActivityTree{Stats: enablePatternsTestStats()}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}

	for _, s := range []string{"aaa", "bbb", "ccc"} {
		insertTestPath(pn, "/tmp/sess-"+s+"/file", at.Stats, false)
	}

	tmp := pn.Files["tmp"]
	assert.Len(t, tmp.Children, 3)
	assert.Equal(t, int64(0), at.Stats.FileNodesMerged)

	at.FinalizePatterns()

	assert.Greater(t, at.Stats.FileNodesMerged, int64(0))
	assert.Contains(t, tmp.Children, "sess-<alpha>", "got: %v", childrenNames(tmp.Children))
}

// /tmp/<num>/subfolder/<num> must collapse while the fixed top-level
// directories stay literal.
func TestFinalizePatterns_PreservesFixedAlphaTopLevelDirs(t *testing.T) {
	at := &ActivityTree{Stats: enablePatternsTestStats()}
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	at.ProcessNodes = []*ProcessNode{pn}

	for _, p := range []string{
		"/tmp/25739/subfolder/2008",
		"/tmp/31282/subfolder/3558",
		"/tmp/449/subfolder/9521",
		"/tmp/13860/subfolder/14273",
		"/etc/hostname",
		"/var/log/messages",
		"/bin/sh",
		"/usr/lib/libc.so.6",
		"/proc/self/status",
	} {
		insertTestPath(pn, p, at.Stats, false)
	}

	at.FinalizePatterns()

	assert.Equal(t, []string{"bin", "etc", "proc", "tmp", "usr", "var"}, childrenNames(pn.Files))
	tmp := pn.Files["tmp"]
	num, ok := tmp.Children["<num>"]
	if !assert.True(t, ok, "got: %v", childrenNames(tmp.Children)) {
		return
	}
	sub, ok := num.Children["subfolder"]
	if !assert.True(t, ok, "got: %v", childrenNames(num.Children)) {
		return
	}
	leaf, ok := sub.Children["<num>"]
	if assert.True(t, ok, "got: %v", childrenNames(sub.Children)) && assert.NotNil(t, leaf.File) {
		assert.Equal(t, "/tmp/<num>/subfolder/<num>", leaf.File.PathnameStr)
	}
}

func TestInsertFileEvent_AnomalyRaisedOnCrossClassVariant(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	for _, id := range []string{"1", "42", "1337", "99999"} {
		insertTestPath(pn, "/var/log/job/"+id, stats, false)
	}
	job := pn.Files["var"].Children["log"].Children["job"]
	mergeChildren(job.Children, 3, stats)
	if !assert.Contains(t, job.Children, "<num>") {
		return
	}

	isNew := insertTestPath(pn, "/var/log/job/malicious_binary", stats, true)
	assert.True(t, isNew, "cross-class candidate must surface as a new entry")
}

func TestInsertFileEvent_SameShapeStillQuiet(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	for _, id := range []string{"1", "42", "1337", "99999"} {
		insertTestPath(pn, "/var/log/job/"+id, stats, false)
	}
	mergeChildren(pn.Files["var"].Children["log"].Children["job"].Children, 3, stats)

	before := stats.FilePatternLookupHits
	isNew := insertTestPath(pn, "/var/log/job/8675309", stats, true)
	assert.False(t, isNew, "same-shape numeric variant must match the trained pattern")
	assert.Greater(t, stats.FilePatternLookupHits, before)
}

// The leaf returned by an insert that triggers a merge must be the live
// pattern node, not the folded-away literal.
func TestInsertFileEvent_ReturnsLiveNodeAfterMerge(t *testing.T) {
	pn := &ProcessNode{
		Files:    make(map[string]*FileNode),
		NodeBase: NewNodeBase(),
	}
	stats := enablePatternsTestStats()

	count := stats.PathPatternConfig().MaxChildren + 1
	var last *NodeBase
	for i := 0; i < count; i++ {
		ev := newTestEvent("/data/" + strconv.Itoa(1000+i))
		_, last = pn.InsertFileEvent(&ev.Open.File, ev, 1, Unknown, stats, false, nil, nil)
	}

	data := pn.Files["data"]
	num, ok := data.Children["<num>"]
	if assert.True(t, ok, "got: %v", childrenNames(data.Children)) {
		assert.Same(t, &num.NodeBase, last)
	}
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

	count := DefaultPathPatternConfig().MaxChildren + 5
	for i := 0; i < count; i++ {
		triple := string(rune('a'+i)) + string(rune('a'+i)) + string(rune('a'+i))
		insertTestPath(pn, "/tmp/sess-"+triple+"/file", stats, false)
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

func newTestEvent(path string) *model.Event {
	return &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Open: model.OpenEvent{File: model.FileEvent{
			IsPathnameStrResolved: true,
			PathnameStr:           path,
		}},
	}
}

func insertTestPath(pn *ProcessNode, path string, stats *Stats, dryRun bool) bool {
	ev := newTestEvent(path)
	isNew, _ := pn.InsertFileEvent(&ev.Open.File, ev, 1, Unknown, stats, dryRun, nil, nil)
	return isNew
}

func childrenNames(children map[string]*FileNode) []string {
	out := make([]string, 0, len(children))
	for name := range children {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
