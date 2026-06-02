// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package cws_codegen registers a Gazelle language for the CWS codegen macros
// declared in //bazel/rules/cws_codegen:defs.bzl.
//
// It scans `//go:generate <gen> ...` directives in regular .go files and emits
// the matching macro call (currently: operators) into the package's BUILD.bazel.
package cws_codegen

import (
	"bufio"
	"flag"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const name = "cws_codegen"

type lang struct {
	language.BaseLang
}

//goland:noinspection GoUnusedExportedFunction
func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string {
	return name
}

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"operators": {
			MatchAttrs:    []string{"output"},
			NonEmptyAttrs: map[string]bool{"output": true},
		},
	}
}

func (*lang) KnownDirectives() []string {
	return []string{name}
}

func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{{
		Name:    "//bazel/rules/cws_codegen:defs.bzl",
		Symbols: []string{"operators"},
	}}
}

func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	if args.File != nil {
		for _, d := range args.File.Directives {
			if d.Key == name && d.Value == "off" {
				return language.GenerateResult{}
			}
		}
	}
	var rules []*rule.Rule
	for _, f := range args.RegularFiles {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		directives, err := readDirectives(filepath.Join(args.Dir, f))
		if err != nil {
			continue
		}
		rules = append(rules, directives...)
	}
	if len(rules) == 0 {
		return language.GenerateResult{}
	}
	return language.GenerateResult{
		Gen:     rules,
		Imports: make([]interface{}, len(rules)),
	}
}

func readDirectives(src string) ([]*rule.Rule, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var directives []*rule.Rule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		goGenerate, ok := strings.CutPrefix(scanner.Text(), "//go:generate ")
		if !ok {
			continue
		}
		if r := parseOperators(goGenerate); r != nil {
			directives = append(directives, r)
		}
	}
	return directives, scanner.Err()
}

func parseOperators(goGenerate string) *rule.Rule {
	fields := strings.Fields(goGenerate)
	var args []string
	for i, f := range fields {
		if path.Base(f) == "operators" {
			args = fields[i+1:]
			break
		}
	}
	if args == nil {
		return nil
	}
	fs := flag.NewFlagSet("operators", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	output := fs.String("output", "", "")
	if err := fs.Parse(args); err != nil {
		return nil
	}
	if *output == "" {
		return nil
	}
	r := rule.NewRule("operators", strings.TrimSuffix(*output, ".go"))
	r.SetAttr("output", *output)
	return r
}
