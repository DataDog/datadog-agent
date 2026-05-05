// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dd_go_test

import (
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func makeGoTestResult(rules ...*rule.Rule) language.GenerateResult {
	imports := make([]interface{}, len(rules))
	for i := range imports {
		imports[i] = []string{"some/import"} // non-nil so forwarding is testable
	}
	return language.GenerateResult{Gen: rules, Imports: imports}
}

func TestReplaceGoTests_NonGoTestPassesThrough(t *testing.T) {
	lib := rule.NewRule("go_library", "lib")
	result := replaceGoTests(makeGoTestResult(lib))

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

	result := replaceGoTests(makeGoTestResult(orig))

	if len(result.Gen) != 1 {
		t.Fatalf("expected 1 gen rule, got %d", len(result.Gen))
	}
	if len(result.Empty) != 1 {
		t.Fatalf("expected 1 empty rule, got %d", len(result.Empty))
	}

	r := result.Gen[0]
	if r.Kind() != "dd_go_test" {
		t.Errorf("expected kind dd_go_test, got %s", r.Kind())
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

	result := replaceGoTests(makeGoTestResult(orig))
	r := result.Gen[0]

	if got := r.AttrStrings("embed"); !stringSlicesEqual(got, []string{":mypkg"}) {
		t.Errorf("embed: got %v, want [:mypkg]", got)
	}
}

func TestReplaceGoTests_ImportsForwarded(t *testing.T) {
	orig := rule.NewRule("go_test", "t")
	result := replaceGoTests(makeGoTestResult(orig))
	if len(result.Imports) != len(result.Gen) {
		t.Errorf("Imports len %d != Gen len %d", len(result.Imports), len(result.Gen))
	}
	if result.Imports[0] == nil {
		t.Error("expected non-nil import forwarded for dd_go_test")
	}
}

func TestReplaceGoTests_MixedRules(t *testing.T) {
	lib := rule.NewRule("go_library", "lib")
	tst := rule.NewRule("go_test", "lib_test")
	bin := rule.NewRule("go_binary", "main")

	result := replaceGoTests(makeGoTestResult(lib, tst, bin))

	if len(result.Gen) != 3 {
		t.Fatalf("expected 3 gen rules, got %d", len(result.Gen))
	}
	if result.Gen[0].Kind() != "go_library" {
		t.Errorf("expected go_library at index 0")
	}
	if result.Gen[1].Kind() != "dd_go_test" {
		t.Errorf("expected dd_go_test at index 1")
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
		if li.Name == "//bazel/rules/dd_go_test:defs.bzl" {
			for _, sym := range li.Symbols {
				if sym == "dd_go_test" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("dd_go_test load not found in ApparentLoads()")
	}
}

func TestConfigure_DirectiveOff(t *testing.T) {
	f := &rule.File{}
	f.Directives = []rule.Directive{{Key: "dd_go_test", Value: "off"}}

	c := &config.Config{Exts: map[string]interface{}{}}
	NewLanguage().(*lang).Configure(c, "some/pkg", f)

	got, ok := c.Exts[extName].(ddGoTestConfig)
	if !ok {
		t.Fatal("expected ddGoTestConfig in c.Exts")
	}
	if got.enabled {
		t.Error("expected enabled=false after directive off")
	}
}

func TestKinds(t *testing.T) {
	kinds := NewLanguage().(*lang).Kinds()
	info, ok := kinds["dd_go_test"]
	if !ok {
		t.Fatal("dd_go_test not in Kinds()")
	}
	if !info.NonEmptyAttrs["embed"] {
		t.Error("expected embed in NonEmptyAttrs")
	}
	if !info.MergeableAttrs["srcs"] {
		t.Error("expected srcs in MergeableAttrs")
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
