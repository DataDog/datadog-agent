// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dd_agent_go_test wraps Gazelle's Go extension and generates
// flavorless build-tag variants for Go tests.
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
const tagSetsDirective = "go_test_tag_sets"

type ddAgentGoTestConfig struct {
	enabled bool
	tagSets []string
}

type lang struct {
	language.Language // embedded Go extension handles all non-test Go rules
}

// NewLanguage returns a Gazelle language extension that wraps the built-in Go extension.
func NewLanguage() language.Language {
	return &lang{Language: goLanguage.NewLanguage()}
}

// Kinds extends the Go extension's kinds with dd_agent_go_test.
func (l *lang) Kinds() map[string]rule.KindInfo {
	kinds := make(map[string]rule.KindInfo, len(l.Language.Kinds())+1)
	for k, v := range l.Language.Kinds() {
		kinds[k] = v
	}
	kinds["dd_agent_go_test"] = rule.KindInfo{
		NonEmptyAttrs: map[string]bool{"embed": true},
		MergeableAttrs: map[string]bool{
			"gotags":          true,
			"include_default": true,
			"srcs":            true,
			"tag_sets":        true,
		},
		ResolveAttrs: map[string]bool{"deps": true},
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

// KnownDirectives registers this extension's directives alongside Go's.
func (l *lang) KnownDirectives() []string {
	return append(l.Language.KnownDirectives(), extName, tagSetsDirective, linuxBPFExtName)
}

// Configure reads the inheritable test conversion and canonical tag-set directives.
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
			switch d.Key {
			case extName:
				cfg.enabled = d.Value != "off"
			case tagSetsDirective:
				cfg.tagSets = parseTagSets(d.Value)
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
		result = l.replaceGoTests(result, args.File, args.Dir, configuredTagSets(args.Config))
	} else {
		// Preserve the go_build_tags extension's behaviour for non-opted packages:
		// every go_test still needs the minimal base test tags.
		for _, r := range result.Gen {
			if r.Kind() == "go_test" {
				r.SetAttr("gotags", BaseTestTags)
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

func configuredTagSets(c *config.Config) []string {
	cfg, ok := c.Exts[extName].(ddAgentGoTestConfig)
	if !ok {
		return nil
	}
	return cfg.tagSets
}

// replaceGoTests converts all go_test rules in result to dd_agent_go_test rules.
// file is the parsed existing BUILD file (may be nil for fresh packages); it
// is consulted to carry over user-managed attrs from any pre-existing go_test
// rule that the merger would otherwise discard along with the rule itself.
//
// "User-managed" is derived from MergeableAttrs: attrs in the Go extension's
// go_test MergeableAttrs are regenerated from source analysis, and attrs in
// dd_agent_go_test's MergeableAttrs are owned by the macro. Everything else is hand-maintained and
// must be carried over.
func (l *lang) replaceGoTests(result language.GenerateResult, file *rule.File, pkgDir string, configuredTagSets []string) language.GenerateResult {
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
	librarySrcs := make(map[string][]string)
	for _, r := range result.Gen {
		if r.Kind() == "go_library" {
			librarySrcs[r.Name()] = r.AttrStrings("srcs")
		}
	}

	for i, r := range result.Gen {
		imp := result.Imports[i]
		if r.Kind() != "go_test" {
			gen = append(gen, r)
			imports = append(imports, imp)
			continue
		}
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

		var embeddedSrcs []string
		for _, embed := range nr.AttrStrings("embed") {
			embeddedSrcs = append(embeddedSrcs, librarySrcs[strings.TrimPrefix(embed, ":")]...)
		}
		if srcs := nr.AttrStrings("srcs"); len(srcs) > 0 {
			includeDefault, tagSets := applicableTagSets(srcs, embeddedSrcs, pkgDir, configuredTagSets)
			if !includeDefault && len(tagSets) == 0 {
				if existingDd, ok := findRule(file, "dd_agent_go_test", r.Name()); ok {
					existingDd.Delete()
				}
				continue
			}
			if !includeDefault {
				nr.SetAttr("include_default", false)
			}
			if len(tagSets) > 0 {
				nr.SetAttr("tag_sets", tagSets)
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
// merge path takes over.
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
		r.DelAttr("include_default")
		r.DelAttr("tag_sets")
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

func parseTagSets(value string) []string {
	return mergeTagSets(strings.Split(value, ","))
}

func mergeTagSets(groups ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, group := range groups {
		for _, tagSet := range group {
			tagSet = normalizeTagSet(tagSet)
			if tagSet == "" || seen[tagSet] {
				continue
			}
			seen[tagSet] = true
			out = append(out, tagSet)
		}
	}
	sort.Strings(out)
	return out
}

func normalizeTagSet(tagSet string) string {
	seen := make(map[string]bool)
	var tags []string
	for _, tag := range strings.Split(tagSet, "+") {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return strings.Join(tags, "+")
}

// platformTokens are GOOS/GOARCH/toolchain identifiers that //go:build expressions
// may reference. We can't resolve them at Gazelle generation time — the target
// platform is chosen later by Bazel via select() — so we treat them as free
// variables and existentially quantify: a tag set matches if there's any
// platform-token assignment that makes the constraint true. (A simple
// "platform tokens are always true" rule misclassifies
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

// applicableTagSets reports whether the default test has sources and returns
// tag combinations that enable additional test or embedded library sources.
func applicableTagSets(testSrcs, librarySrcs []string, pkgDir string, configuredTagSets []string) (bool, []string) {
	includeDefault, testTagSets := sourceTagSets(testSrcs, pkgDir, configuredTagSets, true)
	_, libraryTagSets := sourceTagSets(librarySrcs, pkgDir, configuredTagSets, false)
	return includeDefault, mergeTagSets(testTagSets, libraryTagSets)
}

func sourceTagSets(srcs []string, pkgDir string, configuredTagSets []string, deriveTagSets bool) (bool, []string) {
	baseTags := make(map[string]bool, len(BaseTestTags))
	for _, tag := range BaseTestTags {
		baseTags[tag] = true
	}

	includeDefault := false
	var tagSets []string
	var expressions []constraint.Expr
	for _, s := range srcs {
		path := s
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkgDir, s)
		}
		e, hasConstraint, err := readBuildConstraint(path)
		if err != nil {
			return true, tagSets
		}
		if !hasConstraint {
			includeDefault = true
			continue
		}
		if canSatisfy(e, baseTags) {
			includeDefault = true
			expressions = append(expressions, e)
			continue
		}
		expressions = append(expressions, e)
		tagSets = append(tagSets, tagSetsForExpression(e, baseTags, configuredTagSets, deriveTagSets)...)
	}
	tagSets = pruneRedundantTagSets(mergeTagSets(tagSets), expressions, baseTags)
	return includeDefault, coalesceRelatedTagSets(tagSets, expressions, baseTags)
}

func tagSetsForExpression(expr constraint.Expr, baseTags map[string]bool, configuredTagSets []string, deriveTagSets bool) []string {
	var matches []string
	configuredTags := make(map[string]bool)
	for _, tagSet := range configuredTagSets {
		tags := strings.Split(tagSet, "+")
		for _, tag := range tags {
			configuredTags[tag] = true
		}
		if canSatisfy(expr, activeTagSet(baseTags, tags)) {
			matches = append(matches, tagSet)
		}
	}
	if len(matches) > 0 || referencesAnyTag(expr, configuredTags) || !deriveTagSets {
		return mergeTagSets(matches)
	}
	return minimalTagSets(expr, baseTags)
}

func referencesAnyTag(expr constraint.Expr, tags map[string]bool) bool {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		return tags[e.Tag]
	case *constraint.NotExpr:
		return referencesAnyTag(e.X, tags)
	case *constraint.AndExpr:
		return referencesAnyTag(e.X, tags) || referencesAnyTag(e.Y, tags)
	case *constraint.OrExpr:
		return referencesAnyTag(e.X, tags) || referencesAnyTag(e.Y, tags)
	default:
		return false
	}
}

func minimalTagSets(expr constraint.Expr, baseTags map[string]bool) []string {
	var referenced []string
	collectAutoTestTags(expr, make(map[string]bool), &referenced)
	sort.Strings(referenced)

	var candidates [][]string
	for mask := 1; mask < (1 << len(referenced)); mask++ {
		active := make(map[string]bool, len(baseTags)+len(referenced))
		for tag := range baseTags {
			active[tag] = true
		}
		var selected []string
		for i, tag := range referenced {
			if mask&(1<<i) != 0 {
				active[tag] = true
				selected = append(selected, tag)
			}
		}
		if canSatisfy(expr, active) {
			candidates = append(candidates, selected)
		}
	}

	var out []string
	for i, candidate := range candidates {
		minimal := true
		for j, other := range candidates {
			if i != j && len(other) < len(candidate) && stringSetContains(candidate, other) {
				minimal = false
				break
			}
		}
		if minimal {
			out = append(out, strings.Join(candidate, "+"))
		}
	}
	return mergeTagSets(out)
}

func collectAutoTestTags(expr constraint.Expr, seen map[string]bool, out *[]string) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		if AutoTestTags[e.Tag] && !seen[e.Tag] {
			seen[e.Tag] = true
			*out = append(*out, e.Tag)
		}
	case *constraint.NotExpr:
		collectAutoTestTags(e.X, seen, out)
	case *constraint.AndExpr:
		collectAutoTestTags(e.X, seen, out)
		collectAutoTestTags(e.Y, seen, out)
	case *constraint.OrExpr:
		collectAutoTestTags(e.X, seen, out)
		collectAutoTestTags(e.Y, seen, out)
	}
}

func stringSetContains(superset, subset []string) bool {
	values := make(map[string]bool, len(superset))
	for _, value := range superset {
		values[value] = true
	}
	for _, value := range subset {
		if !values[value] {
			return false
		}
	}
	return true
}

func pruneRedundantTagSets(tagSets []string, expressions []constraint.Expr, baseTags map[string]bool) []string {
	var out []string
	for i, candidate := range tagSets {
		candidateTags := strings.Split(candidate, "+")
		candidateActive := activeTagSet(baseTags, candidateTags)
		redundant := false
		for j, other := range tagSets {
			if i == j {
				continue
			}
			otherTags := strings.Split(other, "+")
			if len(otherTags) <= len(candidateTags) || !stringSetContains(otherTags, candidateTags) {
				continue
			}
			otherActive := activeTagSet(baseTags, otherTags)
			coversSameSources := true
			for _, expr := range expressions {
				if canSatisfy(expr, candidateActive) && !canSatisfy(expr, otherActive) {
					coversSameSources = false
					break
				}
			}
			if coversSameSources {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, candidate)
		}
	}
	return out
}

func activeTagSet(baseTags map[string]bool, tags []string) map[string]bool {
	active := make(map[string]bool, len(baseTags)+len(tags))
	for tag := range baseTags {
		active[tag] = true
	}
	for _, tag := range tags {
		active[tag] = true
	}
	return active
}

func coalesceRelatedTagSets(tagSets []string, expressions []constraint.Expr, baseTags map[string]bool) []string {
	tagSets = append([]string(nil), tagSets...)
	for {
		merged := false
		for i := 0; i < len(tagSets) && !merged; i++ {
			left := strings.Split(tagSets[i], "+")
			for j := i + 1; j < len(tagSets); j++ {
				right := strings.Split(tagSets[j], "+")
				if !stringSetsIntersect(left, right) {
					continue
				}
				union := normalizeTagSet(tagSets[i] + "+" + tagSets[j])
				unionActive := activeTagSet(baseTags, strings.Split(union, "+"))
				leftActive := activeTagSet(baseTags, left)
				rightActive := activeTagSet(baseTags, right)
				preservesCoverage := true
				for _, expr := range expressions {
					wasCovered := canSatisfy(expr, leftActive) || canSatisfy(expr, rightActive)
					if wasCovered && !canSatisfy(expr, unionActive) {
						preservesCoverage = false
						break
					}
				}
				if !preservesCoverage {
					continue
				}
				tagSets[i] = union
				tagSets = append(tagSets[:j], tagSets[j+1:]...)
				merged = true
				break
			}
		}
		if !merged {
			return mergeTagSets(tagSets)
		}
	}
}

func stringSetsIntersect(left, right []string) bool {
	values := make(map[string]bool, len(left))
	for _, value := range left {
		values[value] = true
	}
	for _, value := range right {
		if values[value] {
			return true
		}
	}
	return false
}

// canSatisfy reports whether expr can evaluate to true given active tags,
// treating each platform/arch token as a free variable: if any
// assignment of true/false to those tokens makes the expression true, return
// true. go1.N tokens are resolved against the Gazelle binary's release tags.
//
// In practice constraints reference at most a handful of platform tokens, so
// enumerating 2^N assignments is fast.
func canSatisfy(expr constraint.Expr, activeTags map[string]bool) bool {
	platforms := collectPlatformTokens(expr)
	if len(platforms) == 0 {
		return expr.Eval(func(t string) bool {
			return activeTags[t] || goReleaseTags[t]
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
			return activeTags[t] || goReleaseTags[t]
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
