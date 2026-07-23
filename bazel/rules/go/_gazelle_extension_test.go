// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dd_agent_go_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/merger"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
)

func makeGoTestResult(rules ...*rule.Rule) language.GenerateResult {
	imports := make([]interface{}, len(rules))
	for i := range imports {
		imports[i] = []string{"some/import"} // non-nil so forwarding is testable
	}
	return language.GenerateResult{Gen: rules, Imports: imports}
}

func newLang() *lang {
	return NewLanguage().(*lang)
}

func TestReplaceGoTests_NonGoTestPassesThrough(t *testing.T) {
	lib := rule.NewRule("go_library", "lib")
	result := newLang().replaceGoTests(makeGoTestResult(lib), nil, "", nil)

	if len(result.Gen) != 1 || result.Gen[0].Kind() != "go_library" {
		t.Errorf("expected go_library to pass through, got %v", result.Gen)
	}
	if len(result.Empty) != 0 {
		t.Errorf("expected no empty rules, got %d", len(result.Empty))
	}
	if len(result.Imports) != 1 {
		t.Errorf("expected imports len 1, got %d", len(result.Imports))
	}
}

func TestReplaceGoTests_SingleGoTest(t *testing.T) {
	orig := rule.NewRule("go_test", "pkg_test")
	orig.SetAttr("embed", []string{":pkg"})
	orig.SetAttr("deps", []string{"//some/dep"})

	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "", nil)

	if len(result.Gen) != 1 {
		t.Fatalf("expected 1 gen rule, got %d", len(result.Gen))
	}
	if len(result.Empty) != 1 {
		t.Fatalf("expected 1 empty rule, got %d", len(result.Empty))
	}

	r := result.Gen[0]
	if r.Kind() != "dd_agent_go_test" {
		t.Errorf("expected kind dd_agent_go_test, got %s", r.Kind())
	}
	if r.Name() != "pkg_test" {
		t.Errorf("expected name pkg_test, got %s", r.Name())
	}
	if result.Empty[0].Kind() != "go_test" || result.Empty[0].Name() != "pkg_test" {
		t.Errorf("empty rule: expected go_test pkg_test, got %s %s", result.Empty[0].Kind(), result.Empty[0].Name())
	}
}

func TestReplaceGoTests_ConfiguredTagSets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg_test.go"), []byte("//go:build zlib\n\npackage x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := rule.NewRule("go_test", "pkg_test")
	orig.SetAttr("srcs", []string{"pkg_test.go"})
	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, dir, []string{"zstd+zlib"})

	got := result.Gen[0].AttrStrings("tag_sets")
	want := []string{"zlib+zstd"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("tag_sets = %v, want %v", got, want)
	}
}

func TestReplaceGoTests_AttrsCarriedOver(t *testing.T) {
	orig := rule.NewRule("go_test", "mytest")
	orig.SetAttr("srcs", []string{"mytest.go"})
	orig.SetAttr("embed", []string{":mypkg"})
	orig.SetAttr("data", []string{"testdata/foo.json"})
	orig.SetAttr("target_compatible_with", []string{"@platforms//os:linux"})

	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "", nil)
	r := result.Gen[0]

	if got := r.AttrStrings("embed"); !stringSlicesEqual(got, []string{":mypkg"}) {
		t.Errorf("embed: got %v, want [:mypkg]", got)
	}
	if got := r.AttrStrings("data"); !stringSlicesEqual(got, []string{"testdata/foo.json"}) {
		t.Errorf("data: got %v, want [testdata/foo.json]", got)
	}
	if got := r.AttrStrings("target_compatible_with"); !stringSlicesEqual(got, []string{"@platforms//os:linux"}) {
		t.Errorf("target_compatible_with: got %v, want [@platforms//os:linux]", got)
	}
}

// TestReplaceGoTests_ExistingAttrsPreserved guards the case where the BUILD
// already contains a go_test with user-managed attrs (data, env, tags, …) that
// the freshly generated go_test does not. Without consulting args.File, those
// attrs would be silently dropped along with the deleted go_test rule.
func TestReplaceGoTests_ExistingAttrsPreserved(t *testing.T) {
	fresh := rule.NewRule("go_test", "mytest")
	fresh.SetAttr("srcs", []string{"mytest.go"})
	fresh.SetAttr("embed", []string{":mypkg"})

	prior := rule.NewRule("go_test", "mytest")
	prior.SetAttr("data", []string{"testdata/foo.json"})
	prior.SetAttr("env", map[string]string{"FOO": "bar"})
	prior.SetAttr("tags", []string{"manual"})
	prior.SetAttr("gotags", []string{"test"})   // dd_agent_go_test owns gotags -> should NOT carry over
	prior.SetAttr("srcs", []string{"stale.go"}) // Gazelle-owned -> should NOT carry over
	file := &rule.File{Rules: []*rule.Rule{prior}}

	result := newLang().replaceGoTests(makeGoTestResult(fresh), file, "", nil)
	r := result.Gen[0]

	if got := r.AttrStrings("data"); !stringSlicesEqual(got, []string{"testdata/foo.json"}) {
		t.Errorf("data: got %v, want [testdata/foo.json]", got)
	}
	if got := r.AttrStrings("tags"); !stringSlicesEqual(got, []string{"manual"}) {
		t.Errorf("tags: got %v, want [manual]", got)
	}
	if r.Attr("env") == nil {
		t.Error("env: expected to be preserved from existing rule")
	}
	if r.Attr("gotags") != nil {
		t.Errorf("gotags: expected to be dropped (dd_agent_go_test-managed), got %v", r.AttrStrings("gotags"))
	}
	if got := r.AttrStrings("srcs"); !stringSlicesEqual(got, []string{"mytest.go"}) {
		t.Errorf("srcs: expected fresh value, got %v", got)
	}
}

// TestReplaceGoTests_KeepCommentPreserved guards against silently dropping
// `# keep` annotations on non-managed attrs when converting go_test to
// dd_agent_go_test. The Go extension's pre-merged rule carries the value but
// not the comment; without explicitly copying from the existing rule, the
// next gazelle run would treat the attribute as freshly generated.
func TestReplaceGoTests_KeepCommentPreserved(t *testing.T) {
	fresh := rule.NewRule("go_test", "mytest")
	fresh.SetAttr("srcs", []string{"mytest.go"})
	fresh.SetAttr("embed", []string{":mypkg"})
	// Simulate gazelle's Go-extension pre-merge: r already has tags, no comment.
	fresh.SetAttr("tags", []string{"manual"})

	prior := rule.NewRule("go_test", "mytest")
	prior.SetAttr("tags", []string{"manual"})
	if list, ok := prior.Attr("tags").(*bzl.ListExpr); ok {
		list.Comment().Suffix = append(list.Comment().Suffix, bzl.Comment{Token: "# keep"})
	} else {
		t.Fatalf("expected tags to be a ListExpr, got %T", prior.Attr("tags"))
	}
	file := &rule.File{Rules: []*rule.Rule{prior}}

	result := newLang().replaceGoTests(makeGoTestResult(fresh), file, "", nil)
	r := result.Gen[0]
	list, ok := r.Attr("tags").(*bzl.ListExpr)
	if !ok {
		t.Fatalf("expected tags to be a ListExpr, got %T", r.Attr("tags"))
	}
	suffix := list.Comment().Suffix
	if len(suffix) != 1 || suffix[0].Token != "# keep" {
		t.Errorf("tags suffix comment: got %v, want [# keep]", suffix)
	}
}

func TestReplaceGoTests_ImportsForwarded(t *testing.T) {
	orig := rule.NewRule("go_test", "t")
	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "", nil)
	if len(result.Imports) != len(result.Gen) {
		t.Errorf("Imports len %d != Gen len %d", len(result.Imports), len(result.Gen))
	}
	if result.Imports[0] == nil {
		t.Error("expected non-nil import forwarded for dd_agent_go_test")
	}
}

func TestReplaceGoTests_MixedRules(t *testing.T) {
	lib := rule.NewRule("go_library", "lib")
	tst := rule.NewRule("go_test", "lib_test")
	bin := rule.NewRule("go_binary", "main")

	result := newLang().replaceGoTests(makeGoTestResult(lib, tst, bin), nil, "", nil)

	if len(result.Gen) != 3 {
		t.Fatalf("expected 3 gen rules, got %d", len(result.Gen))
	}
	if result.Gen[0].Kind() != "go_library" {
		t.Errorf("expected go_library at index 0")
	}
	if result.Gen[1].Kind() != "dd_agent_go_test" {
		t.Errorf("expected dd_agent_go_test at index 1")
	}
	if result.Gen[2].Kind() != "go_binary" {
		t.Errorf("expected go_binary at index 2")
	}
	if len(result.Empty) != 1 || result.Empty[0].Kind() != "go_test" {
		t.Errorf("expected exactly 1 go_test in empty")
	}
}

// fakeGoLang stubs the embedded Go extension so GenerateRules can be tested
// against a chosen GenerateResult without a real source-file scan.
type fakeGoLang struct {
	language.BaseLang
	result language.GenerateResult
}

func (f *fakeGoLang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"go_test": {
			MergeableAttrs: map[string]bool{"srcs": true, "embed": true},
			ResolveAttrs:   map[string]bool{"deps": true},
		},
	}
}

func (f *fakeGoLang) GenerateRules(language.GenerateArgs) language.GenerateResult {
	return f.result
}

// TestGenerateRules_OffDirectiveRevertsExistingConversion reproduces the
// observed bug: a package that was previously converted to dd_agent_go_test
// (an on-disk dd_agent_go_test rule exists) has its directive flipped back to
// "off". GenerateRules is Gazelle's real entry point; running its output
// through the same two-phase merge Gazelle itself performs must leave the
// package with a plain go_test, not a stale dd_agent_go_test rule that the
// merge can't reconcile because the kinds differ.
func TestGenerateRules_OffDirectiveRevertsExistingConversion(t *testing.T) {
	old := rule.NewRule("dd_agent_go_test", "pkg_test")
	old.SetAttr("srcs", []string{"pkg_test.go"})
	old.SetAttr("embed", []string{":pkg"})
	file := rule.EmptyFile("BUILD.bazel", "some/pkg")
	old.Insert(file)

	fresh := rule.NewRule("go_test", "pkg_test")
	fresh.SetAttr("srcs", []string{"pkg_test.go"})
	fresh.SetAttr("embed", []string{":pkg"})

	l := &lang{Language: &fakeGoLang{result: language.GenerateResult{
		Gen:     []*rule.Rule{fresh},
		Imports: []interface{}{nil},
	}}}
	c := &config.Config{Exts: map[string]interface{}{extName: ddAgentGoTestConfig{enabled: false}}}

	result := l.GenerateRules(language.GenerateArgs{Config: c, File: file})
	merger.MergeFile(file, result.Empty, result.Gen, merger.PreResolve, l.Kinds(), nil)

	if len(file.Rules) != 1 {
		t.Fatalf("expected 1 rule after merge, got %d: %v", len(file.Rules), file.Rules)
	}
	if file.Rules[0].Kind() != "go_test" {
		t.Errorf("expected go_test after reverting, got %s", file.Rules[0].Kind())
	}
}

// TestGenerateRules_OffDirectiveNoOpWithoutExistingConversion guards the
// already-working case (e.g. cmd/cluster-agent/subcommands/coverage): a
// package that was never converted keeps its plain go_test untouched when
// "off" is (still) in effect.
func TestGenerateRules_OffDirectiveNoOpWithoutExistingConversion(t *testing.T) {
	existing := rule.NewRule("go_test", "pkg_test")
	existing.SetAttr("srcs", []string{"pkg_test.go"})
	file := rule.EmptyFile("BUILD.bazel", "some/pkg")
	existing.Insert(file)

	fresh := rule.NewRule("go_test", "pkg_test")
	fresh.SetAttr("srcs", []string{"pkg_test.go"})

	l := &lang{Language: &fakeGoLang{result: language.GenerateResult{
		Gen:     []*rule.Rule{fresh},
		Imports: []interface{}{nil},
	}}}
	c := &config.Config{Exts: map[string]interface{}{extName: ddAgentGoTestConfig{enabled: false}}}

	result := l.GenerateRules(language.GenerateArgs{Config: c, File: file})
	merger.MergeFile(file, result.Empty, result.Gen, merger.PreResolve, l.Kinds(), nil)

	if len(file.Rules) != 1 || file.Rules[0].Kind() != "go_test" {
		t.Errorf("expected the untouched go_test to survive, got %v", file.Rules)
	}
}

// TestGenerateRules_OffDirectiveRespectsKeptRule guards against deleting a
// whole-rule `# keep` dd_agent_go_test out from under the user: the direct
// Delete() used to revert a stale rule bypasses MergeFile's own ShouldKeep()
// check, so without an explicit guard a hand-protected rule would vanish the
// moment its package's directive flips to off.
func TestGenerateRules_OffDirectiveRespectsKeptRule(t *testing.T) {
	file, err := rule.LoadData("BUILD.bazel", "some/pkg", []byte(`
dd_agent_go_test(
    name = "pkg_test",
    srcs = ["pkg_test.go"],
    embed = [":pkg"],
)  # keep
`))
	if err != nil {
		t.Fatalf("LoadData: %v", err)
	}
	if !file.Rules[0].ShouldKeep() {
		t.Fatalf("expected fixture rule to be marked keep")
	}

	fresh := rule.NewRule("go_test", "pkg_test")
	fresh.SetAttr("srcs", []string{"pkg_test.go"})
	fresh.SetAttr("embed", []string{":pkg"})

	l := &lang{Language: &fakeGoLang{result: language.GenerateResult{
		Gen:     []*rule.Rule{fresh},
		Imports: []interface{}{nil},
	}}}
	c := &config.Config{Exts: map[string]interface{}{extName: ddAgentGoTestConfig{enabled: false}}}

	result := l.GenerateRules(language.GenerateArgs{Config: c, File: file})
	merger.MergeFile(file, result.Empty, result.Gen, merger.PreResolve, l.Kinds(), nil)

	if len(file.Rules) != 1 || file.Rules[0].Kind() != "dd_agent_go_test" {
		t.Errorf("expected the kept dd_agent_go_test rule to survive untouched, got %v", file.Rules)
	}
}

// TestGenerateRules_OffDirectiveSurvivesResolveWithKeptDep replays the full
// three-step pipeline (PreResolve merge, Resolve, PostResolve merge) against
// an existing dd_agent_go_test rule with a `# keep`-marked dep, the same
// scenario TestDepsMerge_KeptItemSurvivesResolveUpdate guards for a package
// that stays dd_agent_go_test. Deleting the old rule during GenerateRules (as
// opposed to mutating it in place) removes it before the PostResolve merge
// ever runs, so whatever the real Go resolver's DelAttr("deps")+SetAttr does
// to the freshly-inserted go_test candidate has no prior rule to reconcile
// kept deps against.
func TestGenerateRules_OffDirectiveSurvivesResolveWithKeptDep(t *testing.T) {
	file, err := rule.LoadData("BUILD.bazel", "some/pkg", []byte(`
dd_agent_go_test(
    name = "pkg_test",
    srcs = ["pkg_test.go"],
    embed = [":pkg"],
    deps = [
        "//kept/dep",  # keep
        "//stale/dep",
    ],
)
`))
	if err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	fresh := rule.NewRule("go_test", "pkg_test")
	fresh.SetAttr("srcs", []string{"pkg_test.go"})
	fresh.SetAttr("embed", []string{":pkg"})

	l := &lang{Language: &fakeGoLang{result: language.GenerateResult{
		Gen:     []*rule.Rule{fresh},
		Imports: []interface{}{nil},
	}}}
	c := &config.Config{Exts: map[string]interface{}{extName: ddAgentGoTestConfig{enabled: false}}}

	result := l.GenerateRules(language.GenerateArgs{Config: c, File: file})
	merger.MergeFile(file, result.Empty, result.Gen, merger.PreResolve, l.Kinds(), nil)

	if len(file.Rules) != 1 {
		t.Fatalf("expected 1 rule after PreResolve merge, got %d: %v", len(file.Rules), file.Rules)
	}
	// Simulate Resolve(): it operates on the generated candidate (fresh), not
	// the existing file rule -- see the real pipeline in cmd/gazelle/update.go,
	// which calls Resolve on v.rules (the GenerateRules output) and only later
	// merges that back into the file at PostResolve.
	fresh.SetAttr("deps", []string{"//fresh/dep"})

	merger.MergeFile(file, nil, []*rule.Rule{fresh}, merger.PostResolve, l.Kinds(), nil)

	got := file.Rules[0].AttrStrings("deps")
	want := []string{"//kept/dep", "//fresh/dep"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("deps after full pipeline: got %v, want %v", got, want)
	}
	if file.Rules[0].Kind() != "go_test" {
		t.Errorf("expected go_test after reverting, got %s", file.Rules[0].Kind())
	}
}

func TestLoads(t *testing.T) {
	mal, ok := NewLanguage().(language.ModuleAwareLanguage)
	if !ok {
		t.Fatal("NewLanguage() does not implement ModuleAwareLanguage")
	}
	loads := mal.ApparentLoads(func(string) string { return "" })
	found := false
	for _, li := range loads {
		if li.Name == "//bazel/rules/go:dd_agent_go_test.bzl" {
			for _, sym := range li.Symbols {
				if sym == "dd_agent_go_test" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("dd_agent_go_test load not found in ApparentLoads()")
	}
}

// TestKnownDirectives ensures the directive is registered so Gazelle's -strict
// mode accepts `# gazelle:dd_agent_go_test`, and that the Go extension's own
// directives are still advertised.
func TestKnownDirectives(t *testing.T) {
	dirs := NewLanguage().(*lang).KnownDirectives()
	found := map[string]bool{}
	for _, d := range dirs {
		found[d] = true
	}
	if !found[extName] {
		t.Errorf("%q not in KnownDirectives: %v", extName, dirs)
	}
	if !found[tagSetsDirective] {
		t.Errorf("%q not in KnownDirectives: %v", tagSetsDirective, dirs)
	}
	if len(dirs) <= 1 {
		t.Errorf("expected Go extension directives to be preserved, got %v", dirs)
	}
}

func TestConfigure_DefaultDisabled(t *testing.T) {
	c := &config.Config{Exts: map[string]interface{}{}}
	NewLanguage().(*lang).Configure(c, "some/pkg", nil)

	got, ok := c.Exts[extName].(ddAgentGoTestConfig)
	if !ok {
		t.Fatal("expected ddAgentGoTestConfig in c.Exts")
	}
	if got.enabled {
		t.Error("expected enabled=false by default (conversion is opt-in)")
	}
}

func TestConfigure_DirectiveOn(t *testing.T) {
	f := &rule.File{}
	f.Directives = []rule.Directive{{Key: "dd_agent_go_test", Value: "on"}}

	c := &config.Config{Exts: map[string]interface{}{}}
	NewLanguage().(*lang).Configure(c, "some/pkg", f)

	if !c.Exts[extName].(ddAgentGoTestConfig).enabled {
		t.Error("expected enabled=true after directive on")
	}
}

func TestConfigure_TagSets(t *testing.T) {
	f := &rule.File{}
	f.Directives = []rule.Directive{{Key: tagSetsDirective, Value: "zstd+zlib, kubeapiserver"}}

	c := &config.Config{Exts: map[string]interface{}{}}
	NewLanguage().(*lang).Configure(c, "some/pkg", f)

	got := c.Exts[extName].(ddAgentGoTestConfig).tagSets
	want := []string{"kubeapiserver", "zlib+zstd"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("tagSets = %v, want %v", got, want)
	}
}

// TestConfigure_DirectiveOff verifies an explicit off wins over an inherited
// "on" (off is also the default, but it must still override).
func TestConfigure_DirectiveOff(t *testing.T) {
	c := &config.Config{Exts: map[string]interface{}{extName: ddAgentGoTestConfig{enabled: true}}}
	f := &rule.File{}
	f.Directives = []rule.Directive{{Key: "dd_agent_go_test", Value: "off"}}
	NewLanguage().(*lang).Configure(c, "some/pkg", f)

	if c.Exts[extName].(ddAgentGoTestConfig).enabled {
		t.Error("expected enabled=false after directive off")
	}
}

// TestConfigure_Inherits verifies the directive is inheritable: a subpackage
// with no directive of its own keeps the value Gazelle cloned in from the
// parent (here, "on"), rather than resetting to the disabled default.
func TestConfigure_Inherits(t *testing.T) {
	l := NewLanguage().(*lang)

	parent := &rule.File{}
	parent.Directives = []rule.Directive{{Key: "dd_agent_go_test", Value: "on"}}
	c := &config.Config{Exts: map[string]interface{}{}}
	l.Configure(c, "some/pkg", parent)

	// Gazelle clones the parent config (carrying c.Exts) before descending; the
	// child then configures with no directive of its own.
	l.Configure(c, "some/pkg/child", nil)

	if !c.Exts[extName].(ddAgentGoTestConfig).enabled {
		t.Error("expected enabled=true inherited from parent")
	}
}

// TestConfigure_OverridesInherited verifies a descendant can disable a subtree
// that an ancestor turned on.
func TestConfigure_OverridesInherited(t *testing.T) {
	l := NewLanguage().(*lang)

	parent := &rule.File{}
	parent.Directives = []rule.Directive{{Key: "dd_agent_go_test", Value: "on"}}
	c := &config.Config{Exts: map[string]interface{}{}}
	l.Configure(c, "some/pkg", parent)

	child := &rule.File{}
	child.Directives = []rule.Directive{{Key: "dd_agent_go_test", Value: "off"}}
	l.Configure(c, "some/pkg/child", child)

	if c.Exts[extName].(ddAgentGoTestConfig).enabled {
		t.Error("expected enabled=false after child disables with off")
	}
}

func TestShouldReplace(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    *config.Config
		want bool
	}{
		{
			name: "default (no config) is disabled",
			c:    &config.Config{Exts: map[string]interface{}{}},
			want: false,
		},
		{
			name: "enabled",
			c: &config.Config{Exts: map[string]interface{}{
				extName: ddAgentGoTestConfig{enabled: true},
			}},
			want: true,
		},
		{
			name: "directive off",
			c: &config.Config{Exts: map[string]interface{}{
				extName: ddAgentGoTestConfig{enabled: false},
			}},
			want: false,
		},
		{
			name: "enabled but map_kind go_test redirects to a custom wrapper",
			c: &config.Config{
				Exts: map[string]interface{}{
					extName: ddAgentGoTestConfig{enabled: true},
				},
				KindMap: map[string]config.MappedKind{
					"go_test": {KindName: "rtloader_go_test", KindLoad: "//rtloader/test:defs.bzl"},
				},
			},
			want: false,
		},
		{
			name: "enabled with map_kind on unrelated kind",
			c: &config.Config{
				Exts: map[string]interface{}{
					extName: ddAgentGoTestConfig{enabled: true},
				},
				KindMap: map[string]config.MappedKind{
					"go_library": {KindName: "my_go_library"},
				},
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldReplace(tc.c); got != tc.want {
				t.Errorf("shouldReplace = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplicableTagSets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, header string) string {
		path := filepath.Join(dir, name)
		body := ""
		if header != "" {
			body = header + "\n\n"
		}
		body += "package x\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return name
	}

	noConstraint := write("plain_test.go", "")
	linuxBpf := write("bpf_test.go", "//go:build linux_bpf")
	requireFips := write("fips_test.go", "//go:build requirefips")
	windowsOnly := write("win_test.go", "//go:build windows")
	platformAlternative := write("platform_alternative_test.go", "//go:build trivy || windows")
	notRequireFips := write("nofips_test.go", "//go:build !requirefips")
	goVersion := write("ver_test.go", "//go:build go1.22")
	tagCombined := write("combo_test.go", "//go:build kubeapiserver && linux")
	twoTags := write("two_tags_test.go", "//go:build trivy && containerd")
	relatedTags := write("related_tags_test.go", "//go:build trivy && docker")
	oneTag := write("one_tag_test.go", "//go:build trivy")
	negativeTag := write("negative_tag_test.go", "//go:build zlib && !zstd")
	compression := write("compression_test.go", "//go:build zlib && zstd")
	alternatives := write("alternatives_test.go", "//go:build docker || containerd")
	depOnly := write("dep_only_test.go", "//go:build trivy_no_javadb")
	taggedLibrary := write("tagged.go", "//go:build kubeapiserver")
	orchestrator := write("orchestrator_test.go", "//go:build orchestrator")
	kubeAPIServerWithoutKubelet := write("kube_no_kubelet_test.go", "//go:build kubeapiserver && !kubelet")
	kubernetesTagSet := "cel+clusterchecks+kubeapiserver+kubelet+orchestrator"
	var manyTagNames []string
	for tag := range AutoTestTags {
		manyTagNames = append(manyTagNames, tag)
	}
	sort.Strings(manyTagNames)
	if len(manyTagNames) <= maxEnumeratedAutoTestTags {
		t.Fatalf("need more than %d auto test tags", maxEnumeratedAutoTestTags)
	}
	manyTagNames = manyTagNames[:maxEnumeratedAutoTestTags+1]
	manyTagSet := strings.Join(manyTagNames, "+")
	manyTags := write("many_tags_test.go", "//go:build "+strings.Join(manyTagNames, " && "))
	manyPositiveTagSet := strings.Join(manyTagNames[:maxEnumeratedAutoTestTags], "+")
	manyTagsWithNegative := write(
		"many_tags_negative_test.go",
		"//go:build "+strings.Join(manyTagNames[:maxEnumeratedAutoTestTags], " && ")+" && !"+manyTagNames[maxEnumeratedAutoTestTags],
	)

	for _, tc := range []struct {
		name              string
		srcs              []string
		librarySrcs       []string
		configuredTagSets []string
		wantDefault       bool
		wantTagSets       []string
	}{
		{
			name:        "unconstrained file uses default only",
			srcs:        []string{noConstraint},
			wantDefault: true,
		},
		{
			name:        "linux_bpf gets focused variant",
			srcs:        []string{linuxBpf},
			wantTagSets: []string{"linux_bpf"},
		},
		{
			name:        "requirefips gets focused variant",
			srcs:        []string{requireFips},
			wantTagSets: []string{"requirefips"},
		},
		{
			name:        "windows-only uses default",
			srcs:        []string{windowsOnly},
			wantDefault: true,
		},
		{
			name:        "feature alternative to platform needs both targets",
			srcs:        []string{platformAlternative},
			wantDefault: true,
			wantTagSets: []string{"trivy"},
		},
		{
			name:        "negative feature uses default",
			srcs:        []string{notRequireFips},
			wantDefault: true,
		},
		{
			name:        "go1.x version constraint uses default",
			srcs:        []string{goVersion},
			wantDefault: true,
		},
		{
			name:        "feature and platform",
			srcs:        []string{tagCombined},
			wantTagSets: []string{"kubeapiserver"},
		},
		{
			name:        "unconstrained and tagged files need both targets",
			srcs:        []string{linuxBpf, noConstraint},
			wantDefault: true,
			wantTagSets: []string{"linux_bpf"},
		},
		{
			name:        "independent tagged files get independent variants",
			srcs:        []string{linuxBpf, requireFips},
			wantTagSets: []string{"linux_bpf", "requirefips"},
		},
		{
			name:        "and expression gets combined variant",
			srcs:        []string{twoTags},
			wantTagSets: []string{"containerd+trivy"},
		},
		{
			name:        "superset covering same sources removes subset",
			srcs:        []string{oneTag, twoTags},
			wantTagSets: []string{"containerd+trivy"},
		},
		{
			name:        "related combinations coalesce",
			srcs:        []string{twoTags, relatedTags},
			wantTagSets: []string{"containerd+docker+trivy"},
		},
		{
			name:        "superset does not remove negative-tag mode",
			srcs:        []string{negativeTag, compression},
			wantTagSets: []string{"zlib", "zlib+zstd"},
		},
		{
			name:        "or expression gets minimal alternatives",
			srcs:        []string{alternatives},
			wantTagSets: []string{"containerd", "docker"},
		},
		{
			name:        "unreadable source does not hide later variants",
			srcs:        []string{"missing_test.go", linuxBpf},
			wantDefault: true,
			wantTagSets: []string{"linux_bpf"},
		},
		{
			name:        "large expression uses bounded combined variant",
			srcs:        []string{manyTags},
			wantTagSets: []string{manyTagSet},
		},
		{
			name:        "large expression preserves negative tags",
			srcs:        []string{manyTagsWithNegative},
			wantTagSets: []string{manyPositiveTagSet},
		},
		{
			name: "dependency-only tag does not create unit test",
			srcs: []string{depOnly},
		},
		{
			name:        "embedded library does not derive a variant",
			srcs:        []string{noConstraint},
			librarySrcs: []string{taggedLibrary},
			wantDefault: true,
		},
		{
			name:              "configured set is canonical for related tags",
			srcs:              []string{orchestrator},
			configuredTagSets: []string{kubernetesTagSet},
			wantTagSets:       []string{kubernetesTagSet},
		},
		{
			name:              "configured set suppresses incompatible partial mode",
			srcs:              []string{kubeAPIServerWithoutKubelet},
			configuredTagSets: []string{kubernetesTagSet},
		},
		{
			name:              "unrelated tags still derive focused variants",
			srcs:              []string{linuxBpf},
			configuredTagSets: []string{kubernetesTagSet},
			wantTagSets:       []string{"linux_bpf"},
		},
		{
			name:              "embedded library selects configured set",
			srcs:              []string{noConstraint},
			librarySrcs:       []string{taggedLibrary},
			configuredTagSets: []string{kubernetesTagSet},
			wantDefault:       true,
			wantTagSets:       []string{kubernetesTagSet},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotDefault, gotTagSets := applicableTagSets(tc.srcs, tc.librarySrcs, dir, tc.configuredTagSets)
			if gotDefault != tc.wantDefault {
				t.Errorf("includeDefault = %v, want %v", gotDefault, tc.wantDefault)
			}
			if !stringSlicesEqual(gotTagSets, tc.wantTagSets) {
				t.Errorf("tagSets = %v, want %v", gotTagSets, tc.wantTagSets)
			}
		})
	}
}

// TestKinds guards deps staying a ResolveAttr: Gazelle's post-Resolve
// MergeFile pass only treats an attr as auto-managed if it's in ResolveAttrs
// (MergeableAttrs governs the earlier pre-resolve pass instead). Without this,
// Resolve's dd_agent_go_test deps never take effect past the first
// conversion. See TestDepsMerge_KeptItemSurvivesResolveUpdate for why
// ResolveAttrs (not MergeableAttrs) is the correct attribute set for this.
func TestKinds(t *testing.T) {
	kinds := NewLanguage().(*lang).Kinds()
	info, ok := kinds["dd_agent_go_test"]
	if !ok {
		t.Fatal("dd_agent_go_test not in Kinds()")
	}
	if !info.NonEmptyAttrs["embed"] {
		t.Error("expected embed in NonEmptyAttrs")
	}
	if !info.MergeableAttrs["srcs"] {
		t.Error("expected srcs in MergeableAttrs")
	}
	if !info.MergeableAttrs["tag_sets"] || !info.MergeableAttrs["include_default"] {
		t.Error("expected flavorless attrs in MergeableAttrs")
	}
	if !info.ResolveAttrs["deps"] {
		t.Error("expected deps in ResolveAttrs")
	}
}

// TestDepsMerge_KeptItemSurvivesResolveUpdate replays Gazelle's real two-phase
// merge (PreResolve, then PostResolve after Resolve sets deps) against a
// dd_agent_go_test rule with a `# keep`-annotated dep, the same pattern used
// for split cgo_align targets (e.g. pkg/collector/corechecks/ebpf/probe/ebpfcheck).
// It guards that a manually kept dep survives while a stale one is dropped and
// a freshly resolved one is added — the same behavior go_test gets for free.
func TestDepsMerge_KeptItemSurvivesResolveUpdate(t *testing.T) {
	kinds := newLang().Kinds()

	old := rule.NewRule("dd_agent_go_test", "pkg_test")
	old.SetAttr("srcs", []string{"pkg_test.go"})
	old.SetAttr("embed", []string{":pkg"})
	old.SetAttr("deps", []string{"//kept/dep", "//stale/dep"})
	list, ok := old.Attr("deps").(*bzl.ListExpr)
	if !ok {
		t.Fatalf("expected deps to be a ListExpr, got %T", old.Attr("deps"))
	}
	list.List[0].Comment().Suffix = append(list.List[0].Comment().Suffix, bzl.Comment{Token: "# keep"})
	file := rule.EmptyFile("BUILD.bazel", "some/pkg")
	old.Insert(file)

	// gen mirrors the rule Gazelle would generate: no deps yet, since Resolve
	// hasn't run at PreResolve time.
	gen := rule.NewRule("dd_agent_go_test", "pkg_test")
	gen.SetAttr("srcs", []string{"pkg_test.go"})
	gen.SetAttr("embed", []string{":pkg"})
	merger.MergeFile(file, nil, []*rule.Rule{gen}, merger.PreResolve, kinds, nil)

	// Simulate Resolve() setting the freshly computed deps on the same rule object.
	gen.SetAttr("deps", []string{"//fresh/dep"})
	merger.MergeFile(file, nil, []*rule.Rule{gen}, merger.PostResolve, kinds, nil)

	got := file.Rules[0].AttrStrings("deps")
	want := []string{"//kept/dep", "//fresh/dep"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("deps after merge: got %v, want %v", got, want)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
