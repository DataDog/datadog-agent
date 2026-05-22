// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package go_stringer

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const name = "go_stringer"

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
		name: {
			MatchAttrs: []string{"output"},
			MergeableAttrs: map[string]bool{ // always replicate //go:generate args
				"build_tags":  true,
				"linecomment": true,
				"src":         true,
				"trimprefix":  true,
				"types":       true,
			},
			NonEmptyAttrs: map[string]bool{"src": true, "types": true},
		},
	}
}

func (*lang) KnownDirectives() []string {
	return []string{name}
}

func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{{
		Name:    fmt.Sprintf("//bazel/rules/%s:defs.bzl", name),
		Symbols: []string{name},
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
	var liveRules []*rule.Rule
	for _, f := range args.RegularFiles {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		directives, err := readDirectives(filepath.Join(args.Dir, f))
		if err != nil {
			continue
		}
		liveRules = append(liveRules, directives...)
	}
	liveOutputs := make(map[string]bool, len(liveRules))
	for _, r := range liveRules {
		liveOutputs[r.AttrString("output")] = true
	}
	var staleRules []*rule.Rule
	if args.File != nil {
		for _, r := range args.File.Rules {
			if r.Kind() == name && !liveOutputs[r.AttrString("output")] {
				staleRules = append(staleRules, rule.NewRule(name, r.Name()))
			}
		}
	}
	return language.GenerateResult{
		Empty:   staleRules,
		Gen:     liveRules,
		Imports: make([]interface{}, len(liveRules)),
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
		if directive := parseDirective(goGenerate, filepath.Base(src)); directive != nil {
			directives = append(directives, directive)
		}
	}
	return directives, scanner.Err()
}

func parseDirective(goGenerate, src string) *rule.Rule {
	fields := strings.Fields(goGenerate)
	var args []string
	for i, field := range fields {
		if strings.HasPrefix(path.Base(field), "stringer") {
			args = fields[i+1:]
			break
		}
	}
	if args == nil {
		return nil
	}

	fs := flag.NewFlagSet("stringer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	linecomment := fs.Bool("linecomment", false, "")
	output := fs.String("output", "", "")
	tags := fs.String("tags", "", "")
	trimprefix := fs.String("trimprefix", "", "")
	type_ := fs.String("type", "", "")
	if err := fs.Parse(args); err != nil {
		return nil
	}
	types := strings.Split(*type_, ",")
	if *output == "" {
		*output = fmt.Sprintf("%s_string.go", strings.ToLower(types[0])) // stringer's default
	}

	directive := rule.NewRule(name, strings.TrimSuffix(*output, ".go"))
	if *linecomment {
		directive.SetAttr("linecomment", *linecomment)
	}
	directive.SetAttr("output", *output)
	directive.SetAttr("src", src)
	if t := strings.Fields(*tags); len(t) > 0 {
		directive.SetAttr("build_tags", t)
	}
	if *trimprefix != "" {
		directive.SetAttr("trimprefix", *trimprefix)
	}
	directive.SetAttr("types", types)
	return directive
}
