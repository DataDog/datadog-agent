// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package go_build_tags is a Gazelle extension that replaces each go_test rule
// with one per-flavor variant. Each variant sets gotags to the full tag set for
// that flavor, causing rules_go to propagate the right build tags to all
// transitive deps during test builds.
package go_build_tags

import (
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bazelbuild/buildtools/build"
)

const extName = "go_build_tags"

// flavorNames mirrors the keys of FLAVOR_UNIT_TEST_TAGS in bazel/flavors/defs.bzl.
var flavorNames = []string{"base", "dogstatsd", "fips", "heroku", "iot"}

type lang struct {
	language.BaseLang
}

func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string { return extName }

// Loads declares the load statement that generated BUILD files need for
// flavor_gotags().
func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{
			Name:    "//bazel/flavors:defs.bzl",
			Symbols: []string{"flavor_gotags"},
			After:   []string{"go_test"},
		},
	}
}

// GenerateRules replaces each go_test from the Go language extension with one
// go_test per flavor. Each per-flavor rule calls flavor_gotags() which handles
// the linux-only select() internally.
func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	var gen []*rule.Rule
	var empty []*rule.Rule

	for _, r := range args.OtherGen {
		if r.Kind() != "go_test" {
			continue
		}
		var flavorTestNames []string
		for _, flavor := range flavorNames {
			nr := rule.NewRule("go_test", r.Name()+"_"+flavor)
			copyAttr(r, nr, "srcs")
			copyAttr(r, nr, "embed")
			copyAttr(r, nr, "deps")
			nr.SetAttr("gotags", flavorGotagsExpr(flavor))
			nr.SetAttr("tags", []string{"flavor_" + flavor})
			gen = append(gen, nr)
			flavorTestNames = append(flavorTestNames, ":"+r.Name()+"_"+flavor)
		}
		ts := rule.NewRule("test_suite", r.Name())
		ts.SetAttr("tests", flavorTestNames)
		gen = append(gen, ts)
		empty = append(empty, rule.NewRule("go_test", r.Name()))
	}

	return language.GenerateResult{
		Gen:     gen,
		Empty:   empty,
		Imports: make([]interface{}, len(gen)),
	}
}

func copyAttr(src, dst *rule.Rule, attr string) {
	if v := src.Attr(attr); v != nil {
		dst.SetAttr(attr, v)
	}
}

func flavorGotagsExpr(flavor string) build.Expr {
	return &build.CallExpr{
		X:    &build.Ident{Name: "flavor_gotags"},
		List: []build.Expr{&build.StringExpr{Value: flavor}},
	}
}
