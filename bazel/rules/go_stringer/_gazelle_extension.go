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
	"slices"
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

func (*lang) Name() string { return name }

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		name: {
			MatchAttrs:    []string{"output"},
			NonEmptyAttrs: map[string]bool{"output": true, "src": true, "types": true},
		},
	}
}

func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{{
		Name:    fmt.Sprintf("//bazel/rules/%s:defs.bzl", name),
		Symbols: []string{name},
	}}
}

func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	goFiles := slices.DeleteFunc(slices.Clone(args.RegularFiles), func(f string) bool {
		return !strings.HasSuffix(f, ".go")
	})
	if len(goFiles) == 0 {
		return language.GenerateResult{}
	}
	var rules []*rule.Rule
	for _, f := range goFiles {
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
		*output = fmt.Sprintf("%s_string.go", strings.ToLower(types[0]))
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
