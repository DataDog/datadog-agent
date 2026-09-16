// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package go_msgp

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

const name = "go_msgp"

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
			MatchAttrs: []string{"out"},
			MergeableAttrs: map[string]bool{ // always replicate //go:generate args
				"io":       true,
				"out_test": true,
			},
			NonEmptyAttrs: map[string]bool{"out": true, "src": true},
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
	return language.GenerateResult{
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
		if strings.HasPrefix(path.Base(field), "msgp") {
			args = fields[i+1:]
			break
		}
	}
	if args == nil {
		return nil
	}

	fs := flag.NewFlagSet("msgp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "")
	output := fs.String("o", "", "")
	ioBool := fs.Bool("io", true, "")
	testsBool := fs.Bool("tests", true, "")
	if err := fs.Parse(args); err != nil {
		return nil
	}
	srcFile := *file
	if srcFile == "" {
		srcFile = src
	}
	out := *output
	if out == "" {
		out = strings.TrimSuffix(srcFile, ".go") + "_gen.go"
	}

	directive := rule.NewRule(name, strings.TrimSuffix(out, ".go"))
	directive.SetAttr("src", srcFile)
	directive.SetAttr("out", out)
	if *ioBool {
		directive.SetAttr("io", true)
	}
	if *testsBool {
		directive.SetAttr("out_test", strings.TrimSuffix(out, ".go")+"_test.go")
	}
	return directive
}
