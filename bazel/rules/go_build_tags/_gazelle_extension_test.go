// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package go_build_tags

import (
	"testing"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
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
	if got := r.AttrStrings("flavors"); !stringSlicesEqual(got, flavorNames) {
		t.Errorf("flavors: got %v, want %v", got, flavorNames)
	}

	if result.Empty[0].Name() != "pkg_test" {
		t.Errorf("empty rule: expected name pkg_test, got %s", result.Empty[0].Name())
	}
}

func TestGenerateRules_AttrsCarriedOver(t *testing.T) {
	orig := rule.NewRule("go_test", "mytest")
	orig.SetAttr("srcs", []string{"mytest.go"})
	orig.SetAttr("embed", []string{":mypkg"})
	orig.SetAttr("deps", []string{"//a:b", "//c:d"})

	result := new(lang).GenerateRules(language.GenerateArgs{OtherGen: []*rule.Rule{orig}})

	r := result.Gen[0]
	if got := r.AttrStrings("embed"); !stringSlicesEqual(got, []string{":mypkg"}) {
		t.Errorf("embed: got %v, want [:mypkg]", got)
	}
	if got := r.AttrStrings("deps"); !stringSlicesEqual(got, []string{"//a:b", "//c:d"}) {
		t.Errorf("deps: got %v, want [//a:b //c:d]", got)
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
	if loads[0].Name != "//bazel/rules/go_build_tags:defs.bzl" {
		t.Errorf("unexpected load name: %s", loads[0].Name)
	}
	found := false
	for _, sym := range loads[0].Symbols {
		if sym == "dd_go_test" {
			found = true
		}
	}
	if !found {
		t.Error("dd_go_test not in Loads symbols")
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
