// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dd_go_test is a Gazelle extension that wraps the built-in Go language
// extension and replaces go_test rules with dd_go_test macro calls that encapsulate
// per-flavor test generation. It must replace (not extend) the built-in Go extension
// in the gazelle_binary languages list.
//
// Add "# gazelle:dd_go_test off" to a BUILD file to keep a plain go_test in that package.
package dd_go_test

import (
	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	goLanguage "github.com/bazelbuild/bazel-gazelle/language/go"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const extName = "dd_go_test"

type ddGoTestConfig struct {
	enabled bool
}

type lang struct {
	language.Language // embedded Go extension handles all non-test Go rules
}

// NewLanguage returns a Gazelle language extension that wraps the built-in Go extension.
func NewLanguage() language.Language {
	return &lang{Language: goLanguage.NewLanguage()}
}

// Kinds extends the Go extension's kinds with dd_go_test.
// srcs and gotags are mergeable so they stay in sync with Go file analysis and
// stale gotags values left over from old go_test rules get cleaned up on the
// next Gazelle run. All other attrs (deps, embed, flavors) are preserved from
// the existing rule.
func (l *lang) Kinds() map[string]rule.KindInfo {
	kinds := make(map[string]rule.KindInfo, len(l.Language.Kinds())+1)
	for k, v := range l.Language.Kinds() {
		kinds[k] = v
	}
	kinds["dd_go_test"] = rule.KindInfo{
		NonEmptyAttrs:  map[string]bool{"embed": true},
		MergeableAttrs: map[string]bool{"srcs": true, "gotags": true, "flavors": true},
	}
	return kinds
}

// ApparentLoads extends the Go extension's load statements with the dd_go_test load.
// The Go extension implements ModuleAwareLanguage; Gazelle calls ApparentLoads when
// the interface is satisfied and never falls back to the deprecated Loads().
func (l *lang) ApparentLoads(moduleToApparentName func(string) string) []rule.LoadInfo {
	var base []rule.LoadInfo
	if mal, ok := l.Language.(language.ModuleAwareLanguage); ok {
		base = mal.ApparentLoads(moduleToApparentName)
	}
	return append(base, rule.LoadInfo{
		Name:    "//bazel/rules/dd_go_test:defs.bzl",
		Symbols: []string{"dd_go_test"},
		After:   []string{"go_test"},
	})
}

// Configure reads the # gazelle:dd_go_test directive from the BUILD file.
// "off" disables the go_test → dd_go_test conversion for this package only.
func (l *lang) Configure(c *config.Config, rel string, f *rule.File) {
	l.Language.Configure(c, rel, f)
	cfg := ddGoTestConfig{enabled: true}
	if f != nil {
		for _, d := range f.Directives {
			if d.Key == "dd_go_test" {
				cfg.enabled = d.Value != "off"
			}
		}
	}
	c.Exts[extName] = cfg
}

// GenerateRules calls the Go extension's GenerateRules and replaces each go_test
// rule in the result with a dd_go_test macro call. The Imports slice is kept in
// sync so the resolver can still add deps to each dd_go_test.
func (l *lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	result := l.Language.GenerateRules(args)
	if cfg, ok := args.Config.Exts[extName].(ddGoTestConfig); ok && !cfg.enabled {
		return result
	}
	return replaceGoTests(result)
}

// replaceGoTests converts all go_test rules in result to dd_go_test rules.
func replaceGoTests(result language.GenerateResult) language.GenerateResult {
	var gen []*rule.Rule
	var empty []*rule.Rule
	var imports []interface{}

	for i, r := range result.Gen {
		var imp interface{}
		if i < len(result.Imports) {
			imp = result.Imports[i]
		}
		if r.Kind() != "go_test" {
			gen = append(gen, r)
			imports = append(imports, imp)
			continue
		}
		nr := rule.NewRule("dd_go_test", r.Name())
		copyAttr(r, nr, "srcs")
		copyAttr(r, nr, "embed")
		gen = append(gen, nr)
		imports = append(imports, imp)
		empty = append(empty, rule.NewRule("go_test", r.Name()))
	}

	return language.GenerateResult{
		Gen:     gen,
		Empty:   append(result.Empty, empty...),
		Imports: imports,
	}
}

// Resolve delegates to the Go extension's resolver. For dd_go_test rules it
// proxies through a temporary go_test rule so the Go extension can resolve imports
// to deps, then copies the resolved deps back.
func (l *lang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	if r.Kind() != "dd_go_test" {
		l.Language.Resolve(c, ix, rc, r, imports, from)
		return
	}
	tmp := rule.NewRule("go_test", r.Name())
	copyAttr(r, tmp, "srcs")
	copyAttr(r, tmp, "embed")
	l.Language.Resolve(c, ix, rc, tmp, imports, from)
	copyAttr(tmp, r, "deps")
}

func copyAttr(src, dst *rule.Rule, attr string) {
	if v := src.Attr(attr); v != nil {
		dst.SetAttr(attr, v)
	}
}
