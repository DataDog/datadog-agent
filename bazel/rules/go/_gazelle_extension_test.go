// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dd_agent_go_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
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
	result := newLang().replaceGoTests(makeGoTestResult(lib), nil, "")

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

	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "")

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

func TestReplaceGoTests_AttrsCarriedOver(t *testing.T) {
	orig := rule.NewRule("go_test", "mytest")
	orig.SetAttr("srcs", []string{"mytest.go"})
	orig.SetAttr("embed", []string{":mypkg"})
	orig.SetAttr("data", []string{"testdata/foo.json"})
	orig.SetAttr("target_compatible_with", []string{"@platforms//os:linux"})

	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "")
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

	result := newLang().replaceGoTests(makeGoTestResult(fresh), file, "")
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

	result := newLang().replaceGoTests(makeGoTestResult(fresh), file, "")
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
	result := newLang().replaceGoTests(makeGoTestResult(orig), nil, "")
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

	result := newLang().replaceGoTests(makeGoTestResult(lib, tst, bin), nil, "")

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
	found := false
	for _, d := range dirs {
		if d == extName {
			found = true
		}
	}
	if !found {
		t.Errorf("%q not in KnownDirectives: %v", extName, dirs)
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

func TestApplicableFlavors(t *testing.T) {
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
	notRequireFips := write("nofips_test.go", "//go:build !requirefips")
	goVersion := write("ver_test.go", "//go:build go1.22")
	tagCombined := write("combo_test.go", "//go:build kubeapiserver && linux")

	for _, tc := range []struct {
		name string
		srcs []string
		want []string
	}{
		{
			name: "unconstrained file => all flavors",
			srcs: []string{noConstraint},
			want: []string{"base", "dogstatsd", "fips", "heroku", "iot"},
		},
		{
			name: "linux_bpf only => no flavor (no flavor's tag set contains linux_bpf)",
			srcs: []string{linuxBpf},
			want: nil,
		},
		{
			name: "requirefips => only fips",
			srcs: []string{requireFips},
			want: []string{"fips"},
		},
		{
			name: "windows-only => all flavors (platform tokens treated as may-match)",
			srcs: []string{windowsOnly},
			want: []string{"base", "dogstatsd", "fips", "heroku", "iot"},
		},
		{
			name: "!requirefips => everything except fips",
			srcs: []string{notRequireFips},
			want: []string{"base", "dogstatsd", "heroku", "iot"},
		},
		{
			name: "go1.x version constraint => all flavors",
			srcs: []string{goVersion},
			want: []string{"base", "dogstatsd", "fips", "heroku", "iot"},
		},
		{
			name: "kubeapiserver && linux => only flavors whose tag set includes kubeapiserver",
			srcs: []string{tagCombined},
			want: []string{"base", "fips"},
		},
		{
			name: "mix: one unconstrained file overrides everything",
			srcs: []string{linuxBpf, noConstraint},
			want: []string{"base", "dogstatsd", "fips", "heroku", "iot"},
		},
		{
			name: "mix: any matching src is enough",
			srcs: []string{linuxBpf, requireFips},
			want: []string{"fips"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applicableFlavors(tc.srcs, dir)
			sort.Strings(got) // applicableFlavors already returns sorted, double-check stability
			if !stringSlicesEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestKinds guards deps staying in MergeableAttrs: Gazelle's post-Resolve
// MergeFile pass treats a non-mergeable attr present on the on-disk rule as
// authoritative, discarding whatever Resolve just computed. Without deps
// marked mergeable here, Resolve's dd_agent_go_test deps never take effect
// past the first conversion.
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
	if !info.MergeableAttrs["deps"] {
		t.Error("expected deps in MergeableAttrs")
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
