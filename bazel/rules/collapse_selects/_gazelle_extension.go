// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package collapse_selects is a Gazelle extension that collapses redundant
// select() branches in go_library, go_test, and go_binary deps attributes.
//
// Gazelle's Go language extension resolves platform build constraints by
// enumerating every known platform and evaluating the constraint for each one.
// For a constraint like //go:build !windows, this produces one branch per
// non-windows platform (typically 15 branches) plus an empty //conditions:default.
// All 15 branches are identical, so the select is far more verbose than needed.
//
// This extension runs after all deps have been resolved and written into the
// build rules, but before BUILD files are written to disk, and collapses the
// deps selects of go_library, go_test, and go_binary rules.
//
// Two collapsing strategies are applied:
//
// Redundant entries: any explicit branch whose value equals //conditions:default
// is removed (it adds no information).
//
// Inversion: when all explicit branches share the same value that differs from
// //conditions:default, the select is inverted. The shared value becomes the
// new //conditions:default, and explicit branches are added only for the
// platforms that were previously covered by //conditions:default (computed as
// knownOSPlatforms minus the explicitly listed platforms). This transformation
// is only applied when inversion produces fewer entries than the original, and
// only when all explicit conditions are OS-level @rules_go//go/platform:{os}
// keys (no GOARCH suffix).
//
// Example: //go:build !windows
//
//	before (15 entries):
//	  deps = select({
//	    "@rules_go//go/platform:aix":    ["@org_golang_x_sys//unix"],
//	    "@rules_go//go/platform:linux":  ["@org_golang_x_sys//unix"],
//	    ...13 more non-windows platforms...
//	    "//conditions:default":          [],
//	  })
//
//	after (2 entries):
//	  deps = select({
//	    "@rules_go//go/platform:windows": [],
//	    "//conditions:default":           ["@org_golang_x_sys//unix"],
//	  })
package collapse_selects

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
)

const extName = "collapse_selects"

// knownOSPlatforms is the complete set of OS platform names that rules_go
// emits as select() condition keys (the "{os}" part of "@rules_go//go/platform:{os}",
// without any GOARCH suffix). When the Go extension resolves a //go:build
// constraint, it generates one select branch per platform in this list.
//
// Keep this list in sync with the platforms emitted by rules_go. If a new
// platform is added to rules_go and is missing here, selects covering all
// platforms except that one will not be collapsed — the extension skips any
// select containing an unrecognized condition key, which is safe.
var knownOSPlatforms = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "illumos",
	"ios", "js", "linux", "netbsd", "openbsd", "osx", "plan9",
	"qnx", "solaris", "windows",
}

var knownOSPlatformSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(knownOSPlatforms))
	for _, p := range knownOSPlatforms {
		m[p] = struct{}{}
	}
	return m
}()

const (
	rulesGoPrefix = "@rules_go//go/platform:"
	condDefault   = "//conditions:default"
)

type lang struct {
	language.BaseLang
	language.BaseLifecycleManager

	existFiles []*rule.File // BUILD files collected during the walk; iterated in AfterResolvingDeps once deps are final
	newRules   []*rule.Rule // rules to insert into new BUILD files; same objects are written to disk
}

func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string { return extName }

// GenerateRules collects references to the files and rules we will process
// later. deps attributes are not populated at this stage — the Go extension
// fills them in during a separate resolution pass. The actual collapsing
// happens in AfterResolvingDeps.
func (l *lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	if args.File != nil {
		// deps is not set yet at this point; it is written by the Go extension's
		// Resolve and then merged back into f.Rules by PostResolve. Store the
		// file so that AfterResolvingDeps can iterate f.Rules once they are final.
		l.existFiles = append(l.existFiles, args.File)
	} else {
		// No existing BUILD file: Gazelle will create one and insert the generated
		// rules as-is (same pointers). Store them directly; they will carry the
		// resolved deps by the time AfterResolvingDeps runs.
		for _, r := range args.OtherGen {
			switch r.Kind() {
			case "go_library", "go_test", "go_binary":
				l.newRules = append(l.newRules, r)
			}
		}
	}
	return language.GenerateResult{}
}

// AfterResolvingDeps is called once all deps have been resolved and written
// into the build rules, but before BUILD files are written to disk.
func (l *lang) AfterResolvingDeps(_ context.Context) {
	for _, f := range l.existFiles {
		for _, r := range f.Rules {
			collapseRuleDeps(r)
		}
	}
	for _, r := range l.newRules {
		collapseRuleDeps(r)
	}
}

func collapseRuleDeps(r *rule.Rule) {
	switch r.Kind() {
	case "go_library", "go_test", "go_binary":
		if expr := r.Attr("deps"); expr != nil {
			if collapsed := collapseExpr(expr); collapsed != nil {
				r.SetAttr("deps", collapsed)
			}
		}
	}
}

// collapseExpr collapses a deps value that is either a bare select() call or a
// concatenation of the form <expr> + select(). The select must be the last
// (rightmost) operand; other shapes (e.g. select() + [...]) are left unchanged.
// Returns the modified expression, or nil if no change was made.
func collapseExpr(expr bzl.Expr) bzl.Expr {
	switch e := expr.(type) {
	case *bzl.CallExpr:
		// collapseSelect returns *bzl.CallExpr. A nil *bzl.CallExpr stored in a
		// bzl.Expr interface is a non-nil interface (typed nil), so the caller's
		// != nil check would pass and cause a panic. Check the concrete pointer
		// before letting it escape into the interface.
		if result := collapseSelect(e); result != nil {
			return result
		}
	case *bzl.BinaryExpr:
		// + is left-associative, so a + b + select() parses as (a + b) + select(),
		// meaning e.Y is always the select regardless of how many terms precede it.
		if e.Op == "+" {
			if call, ok := e.Y.(*bzl.CallExpr); ok {
				if collapsed := collapseSelect(call); collapsed != nil {
					return &bzl.BinaryExpr{X: e.X, Op: "+", Y: collapsed}
				}
			}
		}
	}
	return nil
}

// collapseSelect collapses a select({...}) call.
// Returns the new call if any entries were collapsed, or nil.
func collapseSelect(call *bzl.CallExpr) *bzl.CallExpr {
	ident, ok := call.X.(*bzl.Ident)
	if !ok || ident.Name != "select" {
		return nil
	}
	if len(call.List) != 1 {
		return nil
	}
	dict, ok := call.List[0].(*bzl.DictExpr)
	if !ok {
		return nil
	}
	newDict := collapseDict(dict)
	if newDict == nil {
		return nil
	}
	return &bzl.CallExpr{X: call.X, List: []bzl.Expr{newDict}}
}

// collapseDict applies the collapsing strategies to a select dict.
// Returns the new dict if any change was made, or nil.
func collapseDict(dict *bzl.DictExpr) *bzl.DictExpr {
	for _, kv := range dict.List {
		if _, ok := kv.Key.(*bzl.StringExpr); !ok {
			// unexpected key type; leave the entire dict untouched
			// to avoid processing an incomplete set of values which could
			// drop some entries
			return nil
		}
	}

	// Locate the //conditions:default entry.
	defaultIdx := -1
	for i, kv := range dict.List {
		if kv.Key.(*bzl.StringExpr).Value == condDefault {
			defaultIdx = i
			break
		}
	}
	if defaultIdx < 0 {
		return nil // no default; nothing safe to collapse
	}

	defaultKV := dict.List[defaultIdx]
	defaultKey := exprKey(defaultKV.Value)

	var nonDefault []*bzl.KeyValueExpr
	for i, kv := range dict.List {
		if i != defaultIdx {
			nonDefault = append(nonDefault, kv)
		}
	}
	if len(nonDefault) == 0 {
		return nil // only a default entry; nothing to do
	}

	// Check whether all non-default entries share the same value.
	firstVK := exprKey(nonDefault[0].Value)
	allSame := true
	for _, kv := range nonDefault[1:] {
		if exprKey(kv.Value) != firstVK {
			allSame = false
			break
		}
	}

	if allSame {
		if firstVK == defaultKey {
			// Every explicit entry equals the default; //conditions:default covers
			// everything, so drop the explicit entries entirely.
			return &bzl.DictExpr{List: []*bzl.KeyValueExpr{defaultKV}, ForceMultiLine: true}
		}
		// All explicit entries share one value that differs from the default.
		// Try to invert: make that value the new default and list only the
		// platforms that previously fell through to the old default.
		if newDict := invertOSSelect(nonDefault, defaultKV); newDict != nil {
			return newDict
		}
		return nil
	}

	// Mixed values: some explicit entries match the default (redundant), some
	// don't. Remove only the redundant ones.
	var keep []*bzl.KeyValueExpr
	for _, kv := range nonDefault {
		if exprKey(kv.Value) != defaultKey {
			keep = append(keep, kv)
		}
	}
	if len(keep) == len(nonDefault) {
		return nil
	}
	keep = append(keep, defaultKV)
	return &bzl.DictExpr{List: keep, ForceMultiLine: true}
}

// invertOSSelect attempts to invert a select where all non-default entries are
// OS-level @rules_go//go/platform:{os} conditions with the same value. It
// replaces the explicit entries with entries for the "missing" platforms (those
// in knownOSPlatforms but not in the explicit list), giving the majority value
// to //conditions:default. Returns nil if inversion would not reduce entry count.
func invertOSSelect(nonDefault []*bzl.KeyValueExpr, defaultKV *bzl.KeyValueExpr) *bzl.DictExpr {
	explicit := make(map[string]struct{}, len(nonDefault))
	for _, kv := range nonDefault {
		platform, ok := extractOSPlatform(kv.Key.(*bzl.StringExpr).Value)
		if !ok {
			return nil // non-OS-only condition; bail out
		}
		explicit[platform] = struct{}{}
	}

	var missing []string
	for _, p := range knownOSPlatforms {
		if _, ok := explicit[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)

	if len(missing) >= len(nonDefault) {
		return nil // inversion would not reduce entries
	}

	newList := make([]*bzl.KeyValueExpr, 0, len(missing)+1)
	for _, p := range missing {
		newList = append(newList, &bzl.KeyValueExpr{
			Key:   &bzl.StringExpr{Value: rulesGoPrefix + p},
			Value: defaultKV.Value,
		})
	}
	newList = append(newList, &bzl.KeyValueExpr{
		Key:   &bzl.StringExpr{Value: condDefault},
		Value: nonDefault[0].Value,
	})
	return &bzl.DictExpr{List: newList, ForceMultiLine: true}
}

// extractOSPlatform checks whether condition is a rules_go OS-only platform key
// of the form "@rules_go//go/platform:{os}" with no architecture suffix (e.g.
// "@rules_go//go/platform:linux" is accepted, "@rules_go//go/platform:linux_amd64"
// is not). Returns the OS name and true on success, empty string and false
// otherwise. Keys not in knownOSPlatforms are rejected so that an unrecognized
// platform causes the caller to bail out rather than silently mishandling it.
func extractOSPlatform(condition string) (string, bool) {
	platform, ok := strings.CutPrefix(condition, rulesGoPrefix)
	if !ok {
		return "", false
	}
	if strings.Contains(platform, "_") {
		return "", false // arch-qualified (e.g. linux_amd64)
	}
	if _, known := knownOSPlatformSet[platform]; !known {
		return "", false
	}
	return platform, true
}

// exprKey returns a canonical string key for a bzl.Expr, used for equality
// comparisons when grouping select branches. For list-of-strings (the common
// case for deps), the key is order-independent.
func exprKey(expr bzl.Expr) string {
	switch e := expr.(type) {
	case *bzl.StringExpr:
		return "s:" + e.Value
	case *bzl.ListExpr:
		items := make([]string, 0, len(e.List))
		for _, item := range e.List {
			items = append(items, exprKey(item))
		}
		sort.Strings(items)
		return "l:[" + strings.Join(items, "|") + "]"
	default:
		return fmt.Sprintf("?:%T", expr)
	}
}
