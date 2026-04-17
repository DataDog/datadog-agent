// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package go_build_tags is a Gazelle extension that ensures every go_test rule
// has gotags = ["test"]. This causes rules_go to apply a configuration
// transition that propagates the "test" build tag to all transitive deps,
// enabling //go:build test files in go_library targets to be compiled when
// building tests but excluded from production builds.
//
// The extension also propagates any additional user-defined build tags found in
// the test source files themselves. For example, if a test file is gated by
// //go:build ec2, the extension adds "ec2" to gotags so rules_go does not
// silently exclude the file, producing a test binary with no tests.
//
// Tags listed in knownLinuxOnlyTags (mirroring tasks/build_tags.py LINUX_ONLY_TAGS)
// are injected only on Linux via a select() expression, matching the behaviour of
// dda inv test which never passes those tags on non-Linux platforms.
package go_build_tags

import (
	"bufio"
	"go/build/constraint"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
)

const extName = "go_build_tags"

type lang struct {
	language.BaseLang
}

func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string { return extName }

// Kinds declares that the gotags attribute of go_test rules is managed by this
// extension. Marking it as mergeable ensures that Gazelle overwrites the
// existing gotags value on each run rather than preserving the old value.
func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"go_test": {
			MergeableAttrs: map[string]bool{
				"gotags": true,
			},
		},
	}
}

// GenerateRules adds gotags to every go_test rule. "test" is always included.
// Additional user-defined build tags found in the test source files are split
// into two buckets:
//   - cross-platform tags are added directly to the list.
//   - Linux-only tags (knownLinuxOnlyTags) are wrapped in a select() so they
//     are only active on Linux, matching the behaviour of dda inv test.
//
// For AND expressions every tag is required, so all branches are collected.
// For OR expressions the left branch is tried first; the right branch is only
// consulted if the left produced no user-defined tags (e.g. the left side is
// entirely system tags). NOT sub-expressions are skipped.
func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	for _, r := range args.OtherGen {
		if r.Kind() != "go_test" {
			continue
		}
		alwaysTags := []string{"test"}
		var linuxOnlyTags []string
		for _, tag := range requiredSourceTags(r.AttrStrings("srcs"), args.Dir) {
			if isLinuxOnlyTag(tag) {
				linuxOnlyTags = appendIfMissing(linuxOnlyTags, tag)
			} else {
				alwaysTags = appendIfMissing(alwaysTags, tag)
			}
		}
		setGoTags(r, alwaysTags, linuxOnlyTags)
	}
	return language.GenerateResult{}
}

// setGoTags sets the gotags attribute on r. When linuxOnlyTags is non-empty the
// generated expression is:
//
//	alwaysTags + select({"@platforms//os:linux": linuxOnlyTags, "//conditions:default": []})
//
// so that only Linux Bazel builds compile the Linux-only gated source files.
func setGoTags(r *rule.Rule, alwaysTags, linuxOnlyTags []string) {
	if len(linuxOnlyTags) == 0 {
		r.SetAttr("gotags", alwaysTags)
		return
	}
	alwaysExprs := make([]bzl.Expr, len(alwaysTags))
	for i, t := range alwaysTags {
		alwaysExprs[i] = &bzl.StringExpr{Value: t}
	}
	linuxExprs := make([]bzl.Expr, len(linuxOnlyTags))
	for i, t := range linuxOnlyTags {
		linuxExprs[i] = &bzl.StringExpr{Value: t}
	}
	r.SetAttr("gotags", &bzl.BinaryExpr{
		X:  &bzl.ListExpr{List: alwaysExprs},
		Op: "+",
		Y: &bzl.CallExpr{
			X: &bzl.Ident{Name: "select"},
			List: []bzl.Expr{
				&bzl.DictExpr{
					List: []*bzl.KeyValueExpr{
						{
							Key:   &bzl.StringExpr{Value: "@platforms//os:linux"},
							Value: &bzl.ListExpr{List: linuxExprs},
						},
						{
							Key:   &bzl.StringExpr{Value: "//conditions:default"},
							Value: &bzl.ListExpr{},
						},
					},
				},
			},
		},
	})
}

// requiredSourceTags collects user-defined build tags from the //go:build
// directives of the given source files. Tags from all files are unioned;
// duplicates are suppressed. Files that cannot be read are silently skipped.
func requiredSourceTags(srcs []string, dir string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, src := range srcs {
		for _, tag := range tagsFromFile(filepath.Join(dir, src)) {
			if _, ok := seen[tag]; !ok {
				seen[tag] = struct{}{}
				result = append(result, tag)
			}
		}
	}
	return result
}

// tagsFromFile returns user-defined build tags from the //go:build directive
// in the given file. Tags are collected from all positive sub-expressions;
// NOT sub-expressions are skipped.
func tagsFromFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break // past the leading comment block; no build constraint present
		}
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return nil
		}
		return positiveUserTags(expr)
	}
	return nil
}

// positiveUserTags walks a constraint expression tree and collects all
// user-defined tags from positive sub-expressions. Both AND and OR branches
// are traversed; NOT sub-expressions are skipped. Duplicates are removed
// before returning.
func positiveUserTags(expr constraint.Expr) []string {
	var raw []string
	walkPositive(expr, &raw)
	seen := make(map[string]struct{}, len(raw))
	result := raw[:0]
	for _, t := range raw {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}

func walkPositive(expr constraint.Expr, tags *[]string) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		if !isSystemTag(e.Tag) && !isExcludedTag(e.Tag) {
			*tags = append(*tags, e.Tag)
		}
	case *constraint.AndExpr:
		walkPositive(e.X, tags)
		walkPositive(e.Y, tags)
	case *constraint.OrExpr:
		// Satisfy the OR with as few tags as possible: recurse into the left
		// branch first. Only fall through to the right if the left branch
		// added no user-defined tags (e.g. the left side is entirely system
		// tags such as "linux"). When the OR is already satisfied by a prior
		// AND branch (e.g. ec2 && (ec2 || kubelet)), the left branch appends
		// a duplicate which still advances len(*tags), so the right branch is
		// naturally skipped; positiveUserTags removes the duplicate.
		before := len(*tags)
		walkPositive(e.X, tags)
		if len(*tags) == before {
			walkPositive(e.Y, tags)
		}
		// constraint.NotExpr: skip
	}
}

// isLinuxOnlyTag reports whether tag must only be injected into gotags on Linux.
// This mirrors LINUX_ONLY_TAGS in tasks/build_tags.py: dda inv test never passes
// these tags on non-Linux platforms, so including them unconditionally in gotags
// would cause non-Linux Bazel builds to compile Linux-only test files.
func isLinuxOnlyTag(tag string) bool {
	_, ok := knownLinuxOnlyTags[tag]
	return ok
}

// knownLinuxOnlyTags mirrors LINUX_ONLY_TAGS from tasks/build_tags.py (line 270).
// Keep in sync when that set changes.
var knownLinuxOnlyTags = map[string]struct{}{
	"crio":      {},
	"jetson":    {},
	"linux_bpf": {},
	"netcgo":    {},
	"nvml":      {},
	"pcap":      {},
	"podman":    {},
	"systemd":   {},
	"trivy":     {},
}

// isExcludedTag reports whether tag should not be injected into gotags at all.
// These are user-defined tags that require special infrastructure or build
// contexts not available in the standard test environment.
func isExcludedTag(tag string) bool {
	_, ok := excludedTags[tag]
	return ok
}

// excludedTags lists user-defined build tags that the extension must not
// auto-inject into go_test gotags. Tags here require infrastructure or build
// contexts (e.g. system-probe) that are not available in the standard test
// environment. Add a tag here when including it in gotags would cause builds
// to fail or tests to require unavailable hardware/drivers.
var excludedTags = map[string]struct{}{
	// npm requires the Windows npm kernel driver; only valid in system-probe builds.
	"npm": {},
}

// isSystemTag reports whether tag is a Go toolchain-managed constraint that
// rules_go handles via platform selection rather than gotags. This covers
// GOOS values, GOARCH values, Go version tags (go1.N), and a handful of
// special compiler/mode tags.
func isSystemTag(tag string) bool {
	if _, ok := knownGOOS[tag]; ok {
		return true
	}
	if _, ok := knownGOARCH[tag]; ok {
		return true
	}
	if strings.HasPrefix(tag, "go1.") {
		return true
	}
	switch tag {
	case "cgo", "gc", "gccgo", "ignore":
		return true
	}
	return false
}

// knownGOOS is the set of valid GOOS values as of Go 1.24.
var knownGOOS = map[string]struct{}{
	"aix": {}, "android": {}, "darwin": {}, "dragonfly": {},
	"freebsd": {}, "hurd": {}, "illumos": {}, "ios": {},
	"js": {}, "linux": {}, "nacl": {}, "netbsd": {},
	"openbsd": {}, "plan9": {}, "solaris": {}, "wasip1": {},
	"windows": {}, "zos": {},
}

// knownGOARCH is the set of valid GOARCH values as of Go 1.24.
var knownGOARCH = map[string]struct{}{
	"386": {}, "amd64": {}, "amd64p32": {}, "arm": {},
	"armbe": {}, "arm64": {}, "arm64be": {}, "loong64": {},
	"mips": {}, "mipsle": {}, "mips64": {}, "mips64le": {},
	"mips64p32": {}, "mips64p32le": {}, "ppc": {}, "ppc64": {},
	"ppc64le": {}, "riscv": {}, "riscv64": {}, "s390": {},
	"s390x": {}, "sparc": {}, "sparc64": {}, "wasm": {},
}

func appendIfMissing(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}
