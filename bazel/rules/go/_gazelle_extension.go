// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dd_agent_go_test is a Gazelle extension that wraps the built-in Go language
// extension and replaces go_test rules with dd_agent_go_test macro calls that encapsulate
// per-flavor test generation. It must replace (not extend) the built-in Go extension
// in the gazelle_binary languages list.
//
// Add "# gazelle:dd_agent_go_test off" to a BUILD file to keep a plain go_test in
// that package and its subpackages;
// "# gazelle:dd_agent_go_test on" re-enables a subtree.
// The directive is inheritable
package dd_agent_go_test

import (
	"bufio"
	"go/build"
	"go/build/constraint"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	goLanguage "github.com/bazelbuild/bazel-gazelle/language/go"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const extName = "dd_agent_go_test"

type ddAgentGoTestConfig struct {
	enabled bool
}

type lang struct {
	language.Language // embedded Go extension handles all non-test Go rules
}

// NewLanguage returns a Gazelle language extension that wraps the built-in Go extension.
func NewLanguage() language.Language {
	return &lang{Language: goLanguage.NewLanguage()}
}

// Kinds extends the Go extension's kinds with dd_agent_go_test. srcs, gotags,
// and flavors are mergeable so each Gazelle run regenerates them from current
// source analysis. deps is a ResolveAttr, matching the built-in go_test kind,
// so Resolve (below) can update it while still respecting `# keep` on
// individual entries. All other attrs (embed, data, ...) are preserved from
// the existing rule.
func (l *lang) Kinds() map[string]rule.KindInfo {
	kinds := make(map[string]rule.KindInfo, len(l.Language.Kinds())+1)
	for k, v := range l.Language.Kinds() {
		kinds[k] = v
	}
	kinds["dd_agent_go_test"] = rule.KindInfo{
		NonEmptyAttrs:  map[string]bool{"embed": true},
		MergeableAttrs: map[string]bool{"srcs": true, "gotags": true, "flavors": true},
		ResolveAttrs:   map[string]bool{"deps": true},
	}
	return kinds
}

// ApparentLoads extends the Go extension's load statements with the dd_agent_go_test load.
// The Go extension implements ModuleAwareLanguage; Gazelle calls ApparentLoads when
// the interface is satisfied and never falls back to the deprecated Loads().
func (l *lang) ApparentLoads(moduleToApparentName func(string) string) []rule.LoadInfo {
	var base []rule.LoadInfo
	if mal, ok := l.Language.(language.ModuleAwareLanguage); ok {
		base = mal.ApparentLoads(moduleToApparentName)
	}
	return append(base, rule.LoadInfo{
		Name:    "//bazel/rules/go:dd_agent_go_test.bzl",
		Symbols: []string{"dd_agent_go_test"},
		After:   []string{"go_test"},
	})
}

// KnownDirectives registers dd_agent_go_test and dd_linux_bpf alongside the Go
// extension's directives so Gazelle's -strict mode accepts them.
func (l *lang) KnownDirectives() []string {
	return append(l.Language.KnownDirectives(), extName, linuxBPFExtName)
}

// Configure reads the # gazelle:dd_agent_go_test directive from the BUILD file.
// "on" enables the go_test → dd_agent_go_test conversion; "off" disables it.
// The setting is inheritable: it applies to this package and all subpackages
// until a descendant overrides it, so a subtree can be toggled from one BUILD
// file. We seed from the parent's config (cloned into c.Exts by Gazelle before
// this runs).
func (l *lang) Configure(c *config.Config, rel string, f *rule.File) {
	l.Language.Configure(c, rel, f)
	// Disabled by default: the conversion is opt-in per subtree via
	// "# gazelle:dd_agent_go_test on" while the repo migrates (ABLD-474).
	cfg := ddAgentGoTestConfig{enabled: false}
	if prev, ok := c.Exts[extName].(ddAgentGoTestConfig); ok {
		cfg = prev
	}
	if f != nil {
		for _, d := range f.Directives {
			if d.Key == "dd_agent_go_test" {
				cfg.enabled = d.Value != "off"
			}
		}
	}
	c.Exts[extName] = cfg
	configureLinuxBPF(c, f)
}

// GenerateRules calls the Go extension's GenerateRules and replaces each go_test
// rule in the result with a dd_agent_go_test macro call. The Imports slice is kept in
// sync so the resolver can still add deps to each dd_agent_go_test.
func (l *lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	result := l.Language.GenerateRules(args)
	if shouldReplace(args.Config) {
		result = l.replaceGoTests(result, args.File, args.Dir)
	} else {
		// Preserve the go_build_tags extension's behaviour for non-opted packages:
		// every go_test still needs gotags = ["test"] so that rules_go's
		// configuration transition propagates the "test" build tag to transitive
		// library deps that use //go:build test.
		for _, r := range result.Gen {
			if r.Kind() == "go_test" {
				addStringToListIfMissing(r, "gotags", "test")
			}
		}
		result = l.revertDdAgentGoTests(result, args.File)
	}
	if linuxBPFEnabled(args.Config) {
		result = l.applyLinuxBPF(result, args)
	}
	return result
}

// shouldReplace decides whether go_test rules in this package should be
// rewritten to dd_agent_go_test. The conversion is off by default, so it
// declines unless explicitly enabled (the # gazelle:dd_agent_go_test on
// directive, inherited or local). It also declines when:
//   - the package has a # gazelle:map_kind go_test <wrapper> directive,
//     because the user has already chosen a different wrapper for go_test
//     (e.g. rtloader_go_test that sets up dlopen runfiles) and dd_agent_go_test
//     doesn't compose with such wrappers.
func shouldReplace(c *config.Config) bool {
	cfg, ok := c.Exts[extName].(ddAgentGoTestConfig)
	if !ok || !cfg.enabled {
		return false
	}
	if _, mapped := c.KindMap["go_test"]; mapped {
		return false
	}
	return true
}

// replaceGoTests converts all go_test rules in result to dd_agent_go_test rules.
// file is the parsed existing BUILD file (may be nil for fresh packages); it
// is consulted to carry over user-managed attrs from any pre-existing go_test
// rule that the merger would otherwise discard along with the rule itself.
//
// "User-managed" is derived from MergeableAttrs: attrs in the Go extension's
// go_test MergeableAttrs are regenerated from source analysis, and attrs in
// dd_agent_go_test's MergeableAttrs are owned by the macro (e.g. gotags is replaced
// by flavor_gotags at expansion time). Everything else is hand-maintained and
// must be carried over.
func (l *lang) replaceGoTests(result language.GenerateResult, file *rule.File, pkgDir string) language.GenerateResult {
	managed := make(map[string]bool)
	for attr := range l.Language.Kinds()["go_test"].MergeableAttrs {
		managed[attr] = true
	}
	for attr := range l.Kinds()["dd_agent_go_test"].MergeableAttrs {
		managed[attr] = true
	}

	existing := make(map[string]*rule.Rule)
	if file != nil {
		for _, r := range file.Rules {
			if r.Kind() == "go_test" {
				existing[r.Name()] = r
			}
		}
	}

	var gen []*rule.Rule
	var empty []*rule.Rule
	var imports []interface{}

	for i, r := range result.Gen {
		imp := result.Imports[i]
		if r.Kind() != "go_test" {
			gen = append(gen, r)
			imports = append(imports, imp)
			continue
		}

		srcs := r.AttrStrings("srcs")
		fl := applicableFlavors(srcs, pkgDir)

		nr := rule.NewRule("dd_agent_go_test", r.Name())
		for _, attr := range r.AttrKeys() {
			copyAttr(r, nr, attr)
		}
		if ex, ok := existing[r.Name()]; ok {
			for _, attr := range ex.AttrKeys() {
				if managed[attr] {
					continue
				}
				// Always prefer ex's expression for non-managed attrs:
				// it carries the user-authored comments (e.g. `# keep`)
				// which the freshly generated rule from r doesn't. The
				// value itself is identical — r was pre-merged from ex by
				// the Go extension — we just lose the comments along the
				// way without this overwrite.
				copyAttr(ex, nr, attr)
			}
		}
		// Mark the original go_test for removal regardless of what follows:
		// we're either replacing it with dd_agent_go_test or dropping the test
		// entirely.
		empty = append(empty, rule.NewRule("go_test", r.Name()))

		// dd_agent_go_test's macro emits one go_test per flavor unconditionally;
		// Starlark can't read source files, so it has no way to tell that a
		// //go:build constraint will filter every src out for a given flavor.
		// Without applicableFlavors here, a package with tests gated by e.g.
		// linux_bpf or e2ecoverage (neither in any flavor's gotags) would
		// compile to five empty test binaries that all report "no tests to
		// run". We do the cross-source analysis at Gazelle time and either
		// drop the rule entirely (no flavor applies) or restrict the macro
		// to the applicable subset.
		if len(srcs) > 0 {
			if len(fl) == 0 {
				if existingDd, ok := findRule(file, "dd_agent_go_test", r.Name()); ok {
					existingDd.Delete()
				}
				continue
			}
			if len(fl) < len(FlavorUnitTestTags) {
				nr.SetAttr("flavors", fl)
			}
		}
		gen = append(gen, nr)
		imports = append(imports, imp)
	}

	return language.GenerateResult{
		Gen:     gen,
		Empty:   append(result.Empty, empty...),
		Imports: imports,
	}
}

// revertDdAgentGoTests is the inverse of replaceGoTests: it runs when a
// package's directive has just flipped from "on" back to "off". Deleting the
// old rule and inserting the fresh go_test candidate as new would lose any
// `# keep`-marked deps, since those only survive a match against an existing
// rule at the PostResolve merge, well after this function returns.
//
// So instead of deleting, change the existing rule's kind to "go_test" in
// place: the rule stays put with everything on it, and the ordinary go_test
// merge path (kind now matches) takes over as if the package had never been
// converted. "flavors" has no go_test equivalent and is dropped; everything
// else carries over untouched.
//
// A whole-rule `# keep` is left as dd_agent_go_test, so the go_test candidate
// fails to match it (different kind) and is dropped rather than duplicated.
func (l *lang) revertDdAgentGoTests(result language.GenerateResult, file *rule.File) language.GenerateResult {
	if file == nil {
		return result
	}
	for _, r := range file.Rules {
		if r.Kind() != "dd_agent_go_test" || r.ShouldKeep() {
			continue
		}
		r.DelAttr("flavors")
		r.SetKind("go_test")
	}
	return result
}

// Resolve delegates to the Go extension's resolver. For dd_agent_go_test rules it
// proxies through a temporary go_test rule so the Go extension can resolve imports
// to deps, then copies the resolved deps back.
func (l *lang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	if r.Kind() != "dd_agent_go_test" {
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

func addStringToListIfMissing(r *rule.Rule, attr, value string) {
	for _, s := range r.AttrStrings(attr) {
		if s == value {
			return
		}
	}
	r.SetAttr(attr, append(r.AttrStrings(attr), value))
}

// findRule locates a rule of the given kind and name in file, if any.
func findRule(file *rule.File, kind, name string) (*rule.Rule, bool) {
	if file == nil {
		return nil, false
	}
	for _, r := range file.Rules {
		if r.Kind() == kind && r.Name() == name {
			return r, true
		}
	}
	return nil, false
}

// allFlavors returns the canonical flavor list (sorted by name) used by the
// dd_agent_go_test macro. Like ALL_FLAVORS in bazel/flavors/defs.bzl, it is
// derived from FlavorUnitTestTags to keep a single source of truth.
func allFlavors() []string {
	out := make([]string, 0, len(FlavorUnitTestTags))
	for f := range FlavorUnitTestTags {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// platformTokens are GOOS/GOARCH/toolchain identifiers that //go:build expressions
// may reference. We can't resolve them at Gazelle generation time — the target
// platform is chosen later by Bazel via select() — so we treat them as free
// variables and existentially quantify: a flavor matches if there's any
// platform-token assignment that makes the constraint true under the flavor's
// tag set. (A simple "platform tokens are always true" rule misclassifies
// negations like //go:build !windows, which should match on every non-Windows
// target.)
var platformTokens = map[string]bool{
	// GOOS
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true,
	"hurd": true, "illumos": true, "ios": true, "js": true, "linux": true,
	"netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
	// GOARCH
	"386": true, "amd64": true, "amd64p32": true, "arm": true, "arm64": true,
	"arm64be": true, "armbe": true, "loong64": true, "mips": true, "mips64": true,
	"mips64le": true, "mips64p32": true, "mips64p32le": true, "mipsle": true,
	"ppc": true, "ppc64": true, "ppc64le": true, "riscv": true, "riscv64": true,
	"s390": true, "s390x": true, "sparc": true, "sparc64": true, "wasm": true,
	// toolchain / meta
	"cgo": true, "gc": true, "gccgo": true, "unix": true,
}

// goReleaseTags is the set of go1.N tokens satisfied by the toolchain running
// the Gazelle binary, taken from go/build's authoritative list. The toolchain
// here is rules_go's pinned SDK (via go.work), so this matches what the actual
// build will see at compile time.
var goReleaseTags = func() map[string]bool {
	m := make(map[string]bool, len(build.Default.ReleaseTags))
	for _, t := range build.Default.ReleaseTags {
		m[t] = true
	}
	return m
}()

// applicableFlavors returns the subset of flavor names under which at least
// one src file compiles, i.e. the flavors where this rule's test binary would
// have any test functions at all. A src with no //go:build line always
// compiles, so its mere presence keeps every flavor: which OTHER files a
// flavor happens to exclude doesn't matter once at least one file remains,
// since a rule's set of runnable tests is fully determined by its own srcs --
// dropping a flavor here can only be justified by "this flavor's test binary
// would be entirely empty," never by "some sibling file needs a different
// flavor," which Go's own build-constraint evaluation already handles per
// file with no help needed. Platform/arch identifiers are treated as free
// variables (existentially quantified, since Bazel resolves them
// per-configuration at build time); go1.N tokens are resolved against the
// Gazelle binary's release tags.
func applicableFlavors(srcs []string, pkgDir string) []string {
	type file struct {
		expr          constraint.Expr
		hasConstraint bool
	}
	files := make([]file, 0, len(srcs))
	for _, s := range srcs {
		path := s
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkgDir, s)
		}
		e, hasConstraint, err := readBuildConstraint(path)
		if err != nil {
			// Be conservative: a file we can't read might match anything,
			// so don't restrict the flavor set on its behalf.
			return allFlavors()
		}
		files = append(files, file{e, hasConstraint})
	}

	var out []string
	for _, flavor := range allFlavors() {
		tagSet := make(map[string]bool, len(FlavorUnitTestTags[flavor]))
		for _, t := range FlavorUnitTestTags[flavor] {
			tagSet[t] = true
		}
		for _, f := range files {
			if !f.hasConstraint || canSatisfy(f.expr, tagSet) {
				out = append(out, flavor)
				break
			}
		}
	}
	return out
}

// canSatisfy reports whether expr can evaluate to true given the flavor's
// tag set, treating each platform/arch token as a free variable: if any
// assignment of true/false to those tokens makes the expression true, return
// true. go1.N tokens are resolved against the Gazelle binary's release tags.
//
// In practice constraints reference at most a handful of platform tokens, so
// enumerating 2^N assignments is fast.
func canSatisfy(expr constraint.Expr, flavorTags map[string]bool) bool {
	platforms := collectPlatformTokens(expr)
	if len(platforms) == 0 {
		return expr.Eval(func(t string) bool {
			return flavorTags[t] || goReleaseTags[t]
		})
	}
	for mask := 0; mask < (1 << len(platforms)); mask++ {
		assign := make(map[string]bool, len(platforms))
		for i, t := range platforms {
			if mask&(1<<i) != 0 {
				assign[t] = true
			}
		}
		ok := expr.Eval(func(t string) bool {
			if v, present := assign[t]; present {
				return v
			}
			return flavorTags[t] || goReleaseTags[t]
		})
		if ok {
			return true
		}
	}
	return false
}

// collectPlatformTokens returns the unique platform/arch tokens referenced by
// expr, in deterministic order. Used by canSatisfy to enumerate truth
// assignments over those tokens.
func collectPlatformTokens(expr constraint.Expr) []string {
	seen := map[string]bool{}
	var walk func(constraint.Expr)
	walk = func(e constraint.Expr) {
		switch x := e.(type) {
		case *constraint.TagExpr:
			if platformTokens[x.Tag] {
				seen[x.Tag] = true
			}
		case *constraint.NotExpr:
			walk(x.X)
		case *constraint.AndExpr:
			walk(x.X)
			walk(x.Y)
		case *constraint.OrExpr:
			walk(x.X)
			walk(x.Y)
		}
	}
	walk(expr)
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// readBuildConstraint returns the parsed //go:build expression from the file's
// header, if any. (nil, false, nil) means the file is readable but has no
// //go:build line.
func readBuildConstraint(path string) (constraint.Expr, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if constraint.IsGoBuild(line) {
			e, err := constraint.Parse(line)
			if err != nil {
				return nil, false, nil
			}
			return e, true, nil
		}
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		// First non-comment, non-blank line: any //go:build must precede it.
		return nil, false, nil
	}
	return nil, false, scanner.Err()
}
