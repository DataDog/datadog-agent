// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package go_build_tags is a Gazelle extension that ensures every go_test rule
// has gotags = ["test"]. This causes rules_go to apply a configuration
// transition that propagates the "test" build tag to all transitive deps,
// enabling //go:build test files in go_library targets to be compiled when
// building tests but excluded from production builds.
package go_build_tags

import (
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const extName = "go_build_tags"

// goTestutilKind is declared so Gazelle deletes stale go_testutil rules left
// over from a previous version of this extension.
const goTestutilKind = "go_testutil"

type lang struct {
	language.BaseLang
}

func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string { return extName }

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		// Ownership declaration only; no new rules are generated.
		// Existing go_testutil rules are deleted by Gazelle during merge.
		goTestutilKind: {},
	}
}

func (*lang) Loads() []rule.LoadInfo { return nil }

// GenerateRules adds gotags = ["test"] to every go_test rule so that
// rules_go's configuration transition propagates the "test" build tag to all
// transitive library deps during test builds.
func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	for _, r := range args.OtherGen {
		if r.Kind() == "go_test" {
			addStringToListIfMissing(r, "gotags", "test")
		}
	}
	return language.GenerateResult{}
}

// addStringToListIfMissing appends value to the named string-list attribute of r
// if it is not already present.
func addStringToListIfMissing(r *rule.Rule, attr, value string) {
	for _, s := range r.AttrStrings(attr) {
		if s == value {
			return
		}
	}
	r.SetAttr(attr, append(r.AttrStrings(attr), value))
}
