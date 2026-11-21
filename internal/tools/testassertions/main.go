// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main implements testassertions, a best-effort static analyzer that
// extracts the assertions performed by a Go test (unit or new-e2e).
//
// It parses the test package (ignoring build constraints, so every platform
// variant is analyzed), finds the requested tests, expands testify suites run
// through e2e.Run/suite.Run, and walks the test bodies recursively through
// helpers, closures and callbacks. It recognizes testify assert/require
// (package functions, suite methods, s.Require()/s.Assert(), assert.New),
// t.Error/t.Fatal and polling assertions (Eventually, EventuallyWithT, Never,
// Condition). The control flow (subtests, if/else, loops, switch) is kept so
// that conditional assertions can be told apart.
//
// Usage:
//
//	testassertions [flags] <package dir | test file> [TestName | Type.TestName ...]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("testassertions", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON instead of a tree")
	list := fs.Bool("list", false, "only list the tests (and the suite methods they run)")
	brief := fs.Bool("brief", false, "only print assertion summaries (no call/origin details)")
	runRe := fs.String("run", "", "regexp selecting tests (top-level functions and suite methods)")
	depth := fs.Int("depth", 8, "maximum helper nesting depth to follow")
	compact := fs.Bool("compact", false, "compact output for tools/LLMs: ASCII indentation, one line per node, values inline")
	maxBytes := fs.Int("max-bytes", 0, "with -compact (implied): fold the deepest levels until the output fits in N bytes")
	follow := fs.String("follow", string(followModule), "which helpers to follow: package, module (same go.mod as the test) or repo")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: testassertions [flags] <package dir | test file> [TestName | Type.TestName ...]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("missing package directory")
	}
	mode := followMode(*follow)
	if mode != followModule && mode != followRepo && mode != followPackage {
		return fmt.Errorf("invalid -follow value %q", *follow)
	}

	target := fs.Arg(0)
	if wd := os.Getenv("BUILD_WORKING_DIRECTORY"); wd != "" && !filepath.IsAbs(target) {
		target = filepath.Join(wd, target) // bazel run
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	st, err := os.Stat(target)
	if err != nil {
		return err
	}
	dir := target
	if !st.IsDir() {
		dir = filepath.Dir(target)
	}
	root, err := findRepoRoot(dir)
	if err != nil {
		return err
	}

	l := newLoader(root)
	pkg := l.load(dir, true)
	ex := newExtractor(l, pkg, mode, *depth)

	tests, err := selectTests(l, pkg, fs.Args()[1:], *runRe)
	if err != nil {
		return err
	}
	if len(tests) == 0 {
		return fmt.Errorf("no test found in %s", dir)
	}
	nodes := make([]*Node, 0, len(tests))
	for _, t := range tests {
		nodes = append(nodes, ex.testNode(t))
	}

	switch {
	case *list:
		printList(os.Stdout, nodes)
	case *compact || *maxBytes > 0:
		if *maxBytes > 0 {
			fmt.Fprint(os.Stdout, renderCompactBudget(nodes, *maxBytes))
		} else {
			fmt.Fprint(os.Stdout, renderCompact(nodes, nil))
		}
	case *jsonOut:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(nodes)
	default:
		p := &treePrinter{w: os.Stdout, brief: *brief}
		for i, n := range nodes {
			if i > 0 {
				fmt.Fprintln(os.Stdout)
			}
			p.print(n)
		}
	}
	return nil
}

// selectTests resolves test names to declarations. Without names, every
// top-level TestXxx function of the package is selected.
func selectTests(l *loader, pkg *pkgInfo, names []string, runRe string) ([]*funcInfo, error) {
	var re *regexp.Regexp
	if runRe != "" {
		var err error
		if re, err = regexp.Compile(runRe); err != nil {
			return nil, err
		}
	}
	isTopLevelTest := func(f *funcInfo) bool {
		return strings.HasPrefix(f.name, "Test") && strings.HasSuffix(f.file.path, "_test.go") &&
			f.decl.Type.Params != nil && len(f.decl.Type.Params.List) == 1
	}
	var topLevel, methods []*funcInfo
	for _, fs := range pkg.funcs {
		for _, f := range fs {
			if isTopLevelTest(f) {
				topLevel = append(topLevel, f)
			}
		}
	}
	for _, ms := range pkg.methods {
		for name, fs := range ms {
			if strings.HasPrefix(name, "Test") {
				methods = append(methods, fs...)
			}
		}
	}
	l.sortFuncs(topLevel)
	l.sortFuncs(methods)

	if len(names) == 0 && re == nil {
		return topLevel, nil
	}
	var out []*funcInfo
	if re != nil {
		for _, f := range append(append([]*funcInfo{}, topLevel...), methods...) {
			if re.MatchString(f.name) || (f.recv != "" && re.MatchString(f.recv+"."+f.name)) {
				out = append(out, f)
			}
		}
	}
	for _, name := range names {
		recv, method := "", name
		if i := strings.LastIndex(name, "."); i >= 0 {
			recv, method = name[:i], name[i+1:]
		}
		var found []*funcInfo
		if recv == "" {
			for _, f := range topLevel {
				if f.name == method {
					found = append(found, f)
				}
			}
		}
		for _, f := range methods {
			if f.name == method && (recv == "" || f.recv == recv) {
				found = append(found, f)
			}
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("test %q not found in %s", name, pkg.dir)
		}
		out = append(out, found...)
	}
	return out, nil
}
