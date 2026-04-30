// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package go_build_tags

import (
	"testing"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bazelbuild/buildtools/build"
)

func TestGenerateRules_Empty(t *testing.T) {
	result := new(lang).GenerateRules(language.GenerateArgs{
		OtherGen: []*rule.Rule{rule.NewRule("go_library", "lib")},
	})
	if len(result.Gen) != 0 || len(result.Empty) != 0 {
		t.Errorf("expected no output for non-go_test input, got Gen=%d Empty=%d", len(result.Gen), len(result.Empty))
	}
}

func TestGenerateRules_SingleGoTest(t *testing.T) {
	orig := rule.NewRule("go_test", "pkg_test")
	orig.SetAttr("embed", []string{":pkg"})
	orig.SetAttr("deps", []string{"//some/dep"})

	result := new(lang).GenerateRules(language.GenerateArgs{OtherGen: []*rule.Rule{orig}})

	// N per-flavor go_test rules + 1 test_suite
	wantGen := len(flavorNames) + 1
	if len(result.Gen) != wantGen {
		t.Fatalf("expected %d gen rules, got %d", wantGen, len(result.Gen))
	}
	if len(result.Empty) != 1 {
		t.Fatalf("expected 1 empty rule, got %d", len(result.Empty))
	}
	if result.Empty[0].Name() != "pkg_test" {
		t.Errorf("empty rule: expected name pkg_test, got %s", result.Empty[0].Name())
	}

	// Per-flavor go_test rules come first.
	for i, flavor := range flavorNames {
		r := result.Gen[i]
		if r.Kind() != "go_test" {
			t.Errorf("Gen[%d]: expected kind go_test, got %s", i, r.Kind())
		}
		wantName := "pkg_test_" + flavor
		if r.Name() != wantName {
			t.Errorf("Gen[%d]: expected name %s, got %s", i, wantName, r.Name())
		}
		if r.Attr("gotags") == nil {
			t.Errorf("Gen[%d]: missing gotags attr", i)
		}
		tags := r.AttrStrings("tags")
		for _, wantTag := range []string{"go_tests", "flavor_" + flavor} {
			if !containsString(tags, wantTag) {
				t.Errorf("Gen[%d]: tags %v missing %q", i, tags, wantTag)
			}
		}
	}

	// test_suite is last.
	ts := result.Gen[len(flavorNames)]
	if ts.Kind() != "test_suite" {
		t.Errorf("last Gen rule: expected kind test_suite, got %s", ts.Kind())
	}
	if ts.Name() != "pkg_test" {
		t.Errorf("test_suite: expected name pkg_test, got %s", ts.Name())
	}
	wantTests := make([]string, len(flavorNames))
	for i, flavor := range flavorNames {
		wantTests[i] = ":pkg_test_" + flavor
	}
	if got := ts.AttrStrings("tests"); !stringSlicesEqual(got, wantTests) {
		t.Errorf("test_suite tests: got %v, want %v", got, wantTests)
	}
}

func TestGenerateRules_AttrsCarriedOver(t *testing.T) {
	orig := rule.NewRule("go_test", "mytest")
	orig.SetAttr("srcs", []string{"mytest.go"})
	orig.SetAttr("embed", []string{":mypkg"})
	orig.SetAttr("deps", []string{"//a:b", "//c:d"})

	result := new(lang).GenerateRules(language.GenerateArgs{OtherGen: []*rule.Rule{orig}})

	for i, flavor := range flavorNames {
		r := result.Gen[i]
		if got := r.AttrStrings("embed"); !stringSlicesEqual(got, []string{":mypkg"}) {
			t.Errorf("%s: embed: got %v, want [:mypkg]", flavor, got)
		}
		if got := r.AttrStrings("deps"); !stringSlicesEqual(got, []string{"//a:b", "//c:d"}) {
			t.Errorf("%s: deps: got %v, want [//a:b //c:d]", flavor, got)
		}
	}
}

func TestGenerateRules_ImportsLenMatchesGen(t *testing.T) {
	orig := rule.NewRule("go_test", "t")
	result := new(lang).GenerateRules(language.GenerateArgs{OtherGen: []*rule.Rule{orig}})
	if len(result.Imports) != len(result.Gen) {
		t.Errorf("Imports len %d != Gen len %d", len(result.Imports), len(result.Gen))
	}
}

func TestLoads(t *testing.T) {
	loads := new(lang).Loads()
	if len(loads) != 1 {
		t.Fatalf("expected 1 LoadInfo, got %d", len(loads))
	}
	if loads[0].Name != "//bazel/flavors:defs.bzl" {
		t.Errorf("unexpected load name: %s", loads[0].Name)
	}
	found := false
	for _, sym := range loads[0].Symbols {
		if sym == "flavor_gotags" {
			found = true
		}
	}
	if !found {
		t.Error("flavor_gotags not in Loads symbols")
	}
}

func TestFlavorGotagsExpr(t *testing.T) {
	expr := flavorGotagsExpr("base")
	call, ok := expr.(*build.CallExpr)
	if !ok {
		t.Fatalf("expected *build.CallExpr, got %T", expr)
	}
	ident, ok := call.X.(*build.Ident)
	if !ok || ident.Name != "flavor_gotags" {
		t.Fatalf("expected call to flavor_gotags, got %v", call.X)
	}
	if len(call.List) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(call.List))
	}
	arg, ok := call.List[0].(*build.StringExpr)
	if !ok || arg.Value != "base" {
		t.Errorf("expected string arg \"base\", got %v", call.List[0])
	}
}

func TestGenerateRules_FlavorTags(t *testing.T) {
	orig := rule.NewRule("go_test", "t")
	result := new(lang).GenerateRules(language.GenerateArgs{OtherGen: []*rule.Rule{orig}})

	for i, flavor := range flavorNames {
		tags := result.Gen[i].AttrStrings("tags")
		for _, wantTag := range []string{"go_tests", "flavor_" + flavor} {
			if !containsString(tags, wantTag) {
				t.Errorf("Gen[%d] (%s): tags %v missing %q", i, flavor, tags, wantTag)
			}
		}
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

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
