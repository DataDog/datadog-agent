// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package go_package_metadata makes Go packages default to their module's pkg:golang metadata.
package go_package_metadata

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazel-contrib/bazel-gazelle/v2/config"
	"github.com/bazel-contrib/bazel-gazelle/v2/label"
	"github.com/bazel-contrib/bazel-gazelle/v2/language"
	"github.com/bazel-contrib/bazel-gazelle/v2/merger"
	"github.com/bazel-contrib/bazel-gazelle/v2/rule"
	languagev1 "github.com/bazelbuild/bazel-gazelle/language"
	"golang.org/x/mod/modfile"
)

const name = "go_package_metadata"

// module is the Go module enclosing a directory.
type module struct {
	path string // empty for a go.mod declaring no module, which still fences off its directory
	rel  string
}

type lang struct {
	used map[string]bool // rels of the modules owning at least one package with Go rules
}

func NewV2() language.Language {
	return &lang{used: map[string]bool{}}
}

func (*lang) Name() string {
	return name
}

func (*lang) KnownDirectives() []string {
	return nil
}

func (*lang) Configure(_ context.Context, args config.ConfigureArgs) error {
	data, err := os.ReadFile(filepath.Join(args.Config.RepoRoot, args.Rel, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	args.Config.Exts[name] = module{path: modfile.ModulePath(data), rel: args.Rel}
	return nil
}

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"package": { // supersedes the visibility extension's, hence its attributes
			MatchAny: true,
			MergeableAttrs: map[string]bool{
				"default_package_metadata": true,
				"default_visibility":       true,
				"features":                 true,
			},
			NonEmptyAttrs: map[string]bool{"default_package_metadata": true}, // for Generate's Empty
		},
		name: { // attributes (e.g. licenses) are left to humans
			MergeableAttrs: map[string]bool{"module": true},
			NonEmptyAttrs:  map[string]bool{"module": true},
		},
	}
}

func (*lang) ApparentLoads(func(string) string) []rule.LoadInfo {
	return []rule.LoadInfo{{Name: "//bazel/rules/go_package_metadata:defs.bzl", Symbols: []string{name}}}
}

// Generate relies on Gazelle's post-order walk: a module root sees whether its packages use it.
func (l *lang) Generate(_ context.Context, args language.GenerateArgs) (language.GenerateResult, error) {
	var res language.GenerateResult
	mod, ok := args.Config.Exts[name].(module)
	ok = ok && mod.path != ""
	if ok && hasGoRules(args.OtherGen) {
		l.used[mod.rel] = true
		pkg := packageRule(args)
		switch {
		case mod.rel != "":
			pkg.SetAttr("default_package_metadata", []string{label.New("", mod.rel, name).Rel("", args.Rel).String()})
			res.Gen = append(res.Gen, pkg)
		// the root module is attached by REPO.bazel's repo(), hence dropping package-level defaults
		case pkg.Attr("default_package_metadata") == nil:
		case len(pkg.AttrKeys()) == 1:
			res.Empty = append(res.Empty, rule.NewRule("package", ""))
		default:
			pkg.DelAttr("default_package_metadata")
			res.Gen = append(res.Gen, pkg)
		}
	}
	r := rule.NewRule(name, name)
	if ok && args.Rel == mod.rel && l.used[mod.rel] {
		r.SetAttr("module", mod.path)
		r.SetPrivateAttr(merger.UnstableInsertIndexKey, metadataIndex(args.File))
		res.Gen = append(res.Gen, r)
	} else { // also deletes the target a module root left behind
		res.Empty = append(res.Empty, r)
	}
	res.Imports = make([]any, len(res.Gen))
	return res, nil
}

func hasGoRules(rules []*rule.Rule) bool {
	for _, r := range rules {
		if strings.Contains(r.Kind(), "go_") {
			return true
		}
	}
	return false
}

func packageRule(args language.GenerateArgs) *rule.Rule {
	r := rule.NewRule("package", "")
	var existing []*rule.Rule
	if args.File != nil {
		existing = append(existing, args.File.Rules...)
	}
	// carry over existing attributes because merging drops the mergeable ones missing here
	for _, e := range append(existing, args.OtherGen...) {
		if e.Kind() == "package" {
			for _, key := range e.AttrKeys() {
				r.SetAttr(key, e.Attr(key))
			}
		}
	}
	r.SetPrivateAttr(merger.UnstableInsertIndexKey, insertIndex(args.File))
	return r
}

// metadataIndex places the module's target right after package(), existing or inserted alongside.
func metadataIndex(f *rule.File) int {
	if f != nil {
		for _, r := range f.Rules {
			if r.Kind() == "package" {
				return r.Index() + 1
			}
		}
	}
	return insertIndex(f)
}

// insertIndex places package() right after the loads, ahead of any target, rule-declared or not.
func insertIndex(f *rule.File) int {
	index := 0
	if f != nil {
		for _, l := range f.Loads {
			index = max(index, l.Index()+1)
		}
	}
	return index
}

// NewLanguage adapts NewV2 for gazelle_binary, which calls NewV2 only since bazel-gazelle#2429.
func NewLanguage() languagev1.Language {
	return &v1{v2: NewV2().(*lang)}
}

type v1 struct {
	languagev1.BaseLang
	v2 *lang
}

func (s *v1) Name() string {
	return s.v2.Name()
}

func (s *v1) KnownDirectives() []string {
	return s.v2.KnownDirectives()
}

func (s *v1) Configure(c *config.Config, rel string, f *rule.File) {
	if err := s.v2.Configure(context.Background(), config.ConfigureArgs{Config: c, Rel: rel, File: f}); err != nil {
		log.Fatal(err)
	}
}

func (s *v1) Kinds() map[string]rule.KindInfo {
	return s.v2.Kinds()
}

func (s *v1) Loads() []rule.LoadInfo {
	return s.v2.ApparentLoads(nil)
}

func (s *v1) ApparentLoads(moduleToApparentName func(string) string) []rule.LoadInfo {
	return s.v2.ApparentLoads(moduleToApparentName)
}

func (s *v1) GenerateRules(args languagev1.GenerateArgs) languagev1.GenerateResult {
	res, err := s.v2.Generate(context.Background(), language.GenerateArgs{
		Config:       args.Config,
		Dir:          args.Dir,
		Rel:          args.Rel,
		File:         args.File,
		Subdirs:      args.Subdirs,
		RegularFiles: args.RegularFiles,
		GenFiles:     args.GenFiles,
		OtherEmpty:   args.OtherEmpty,
		OtherGen:     args.OtherGen,
	})
	if err != nil {
		log.Fatal(err)
	}
	return languagev1.GenerateResult{Gen: res.Gen, Empty: res.Empty, Imports: res.Imports, RelsToIndex: res.RelsToIndex}
}
