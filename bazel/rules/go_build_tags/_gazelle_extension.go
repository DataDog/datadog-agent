// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package go_build_tags is a Gazelle extension that replaces each go_test rule
// with a dd_go_test macro call that encapsulates per-flavor test generation.
package go_build_tags

import (
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
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

// Loads declares the load statement that generated BUILD files need for dd_go_test.
func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{
			Name:    "//bazel/rules/go_build_tags:defs.bzl",
			Symbols: []string{"dd_go_test"},
			After:   []string{"go_test"},
		},
	}
}

// GenerateRules replaces each go_test from the Go language extension with a
// dd_go_test that handles per-flavor gotags and tagging internally.
func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	var gen []*rule.Rule
	var empty []*rule.Rule

	for _, r := range args.OtherGen {
		if r.Kind() != "go_test" {
			continue
		}
		nr := rule.NewRule("dd_go_test", r.Name())
		copyAttr(r, nr, "srcs")
		copyAttr(r, nr, "embed")
		copyAttr(r, nr, "deps")
		nr.SetAttr("flavors", flavorNames)
		gen = append(gen, nr)
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
