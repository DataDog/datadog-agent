// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ebpf_c keeps eBPF C dependencies honest: it generates one cc_library
// per header and points every eBPF program at just the headers its source
// actually includes, so that nothing is declared to clang without being read.
//
// All libraries of a tree are emitted into the tree's root package and named
// hdr/<path relative to it>, e.g. //pkg/network/ebpf/c:hdr/protocols/tls/tags.
// Giving each subdirectory its own package would force strip_include_prefix,
// which routes clang through _virtual_includes and breaks the unused-input
// bookkeeping in //bazel/rules/ebpf:ebpf.bzl, which matches declared input
// paths against the paths clang writes into its depfile.
package ebpf_c

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const langName = "ebpf_c"

const hdrKind = "cc_library"

// progKinds are the eBPF program macros whose deps this extension owns. Only
// deps is touched; core, extra_flags and the rest stay hand-written.
var progKinds = []string{"ebpf_prog", "ebpf_program_suite"}

// hdrPrefix namespaces the generated libraries. Naming them after the bare
// header would collide with same-named eBPF programs (offset-guess.h next to
// ebpf_program_suite "offset-guess"), and Gazelle drops such a rule silently.
const hdrPrefix = "hdr/"

// ruleOwnedIncludes are headers the _ebpf_prog rule supplies itself instead of
// through deps, because whether clang reads them depends on flags the rule
// sets rather than on anything visible in the build graph: vmlinux.h is only
// reachable under -DCOMPILE_CORE. Keep in sync with //bazel/rules/ebpf:ebpf.bzl.
var ruleOwnedIncludes = map[string]bool{"vmlinux.h": true}

var includeRe = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*include[ \t]+"([^"]+)"`)

// includeRef is one #include, paired with the directory it resolves against.
type includeRef struct {
	// dir is the repo-relative directory of the including file, which clang
	// searches before the include roots. For a program it is the directory of
	// its src, which is not always the package the program is declared in.
	dir string
	inc string
}

// cfg is the inheritable per-directory state of the extension.
type cfg struct {
	// enabled reports whether this directory belongs to an eBPF C tree.
	enabled bool
	// root is the tree root package that owns the generated header libraries.
	root string
}

type lang struct {
	language.BaseLang
}

//goland:noinspection GoUnusedExportedFunction
func NewLanguage() language.Language {
	return &lang{}
}

func (*lang) Name() string {
	return langName
}

func (*lang) KnownDirectives() []string {
	return []string{langName}
}

func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{Name: "@rules_cc//cc:cc_library.bzl", Symbols: []string{hdrKind}},
		{Name: "//bazel/rules/ebpf:ebpf.bzl", Symbols: progKinds},
	}
}

func (*lang) Kinds() map[string]rule.KindInfo {
	kinds := map[string]rule.KindInfo{
		hdrKind: {
			MatchAttrs:    []string{"hdrs"},
			NonEmptyAttrs: map[string]bool{"hdrs": true},
			// hdrs and includes are fully derived from the source tree; deps is
			// derived from #include edges during the resolve phase.
			MergeableAttrs: map[string]bool{"hdrs": true, "includes": true},
			ResolveAttrs:   map[string]bool{"deps": true},
		},
	}
	for _, kind := range progKinds {
		kinds[kind] = rule.KindInfo{
			// src is never rewritten, but naming it keeps a program from being
			// considered empty and deleted when it resolves to no deps.
			NonEmptyAttrs: map[string]bool{"src": true},
			ResolveAttrs:  map[string]bool{"deps": true},
		}
	}
	return kinds
}

func getCfg(c *config.Config) *cfg {
	if existing, ok := c.Exts[langName].(*cfg); ok {
		return existing
	}
	return &cfg{}
}

func (*lang) Configure(c *config.Config, rel string, f *rule.File) {
	parent := getCfg(c)
	current := &cfg{enabled: parent.enabled, root: parent.root}
	if f != nil {
		for _, d := range f.Directives {
			if d.Key != langName {
				continue
			}
			switch d.Value {
			case "on":
				current.enabled, current.root = true, rel
			case "off":
				current.enabled, current.root = false, ""
			}
		}
	}
	c.Exts[langName] = current
}

// ownedHeaders returns every header the root package owns: the ones directly in
// dir plus those in subdirectories that are not Bazel packages of their own.
func ownedHeaders(dir string) []string {
	var headers []string
	var walk func(sub string)
	walk = func(sub string) {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			return
		}
		for _, entry := range entries {
			rel := path.Join(sub, entry.Name())
			if entry.IsDir() {
				if _, err := os.Stat(filepath.Join(dir, rel, "BUILD.bazel")); err == nil {
					continue // a package of its own, not ours to own
				}
				walk(rel)
				continue
			}
			if strings.HasSuffix(entry.Name(), ".h") {
				headers = append(headers, rel)
			}
		}
	}
	walk("")
	sort.Strings(headers)
	return headers
}

func quotedIncludes(file string) []string {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	matches := includeRe.FindAllStringSubmatch(string(content), -1)
	includes := make([]string, 0, len(matches))
	for _, m := range matches {
		includes = append(includes, m[1])
	}
	return includes
}

// headerGroups partitions a tree's headers so that headers which include each
// other end up in one library. C tolerates such cycles thanks to include
// guards, Bazel does not, so each strongly connected component of the intra-
// tree include graph has to be a single target.
func headerGroups(headers []string, includes map[string][]string) [][]string {
	owned := make(map[string]bool, len(headers))
	for _, h := range headers {
		owned[h] = true
	}
	edges := make(map[string][]string, len(headers))
	for _, h := range headers {
		for _, inc := range includes[h] {
			for _, candidate := range []string{path.Join(path.Dir(h), inc), inc} {
				if owned[candidate] && candidate != h {
					edges[h] = append(edges[h], candidate)
					break
				}
			}
		}
	}

	var groups [][]string
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	counter := 0

	var visit func(string)
	visit = func(v string) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range edges[v] {
			if _, seen := index[w]; !seen {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}
		if low[v] != index[v] {
			return
		}
		var group []string
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			group = append(group, w)
			if w == v {
				break
			}
		}
		sort.Strings(group)
		groups = append(groups, group)
	}
	for _, h := range headers {
		if _, seen := index[h]; !seen {
			visit(h)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return groups
}

// srcLocation returns the repo-relative directory and absolute path of a
// program's src, which may be a label pointing into another package.
func srcLocation(repoRoot, rel, src string) (string, string) {
	dir, name := rel, src
	if trimmed, ok := strings.CutPrefix(src, "//"); ok {
		pkg, file, found := strings.Cut(trimmed, ":")
		if !found {
			pkg, file = path.Dir(trimmed), path.Base(trimmed)
		}
		dir, name = pkg, file
	} else {
		name = strings.TrimPrefix(src, ":")
	}
	return dir, filepath.Join(repoRoot, filepath.FromSlash(dir), filepath.FromSlash(name))
}

func (*lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	conf := getCfg(args.Config)
	if !conf.enabled {
		return language.GenerateResult{}
	}

	var rules []*rule.Rule
	var imports []interface{}
	live := map[string]bool{}

	// Only the tree root emits header libraries; subdirectories stay
	// package-free so headers keep their real source paths on the clang
	// command line.
	if conf.root == args.Rel {
		headers := ownedHeaders(args.Dir)
		includes := make(map[string][]string, len(headers))
		for _, header := range headers {
			includes[header] = quotedIncludes(filepath.Join(args.Dir, header))
		}
		for _, group := range headerGroups(headers, includes) {
			name := hdrPrefix + strings.TrimSuffix(group[0], ".h")
			live[name] = true
			r := rule.NewRule(hdrKind, name)
			r.SetAttr("hdrs", group)
			r.SetAttr("includes", []string{"."})
			r.SetAttr("visibility", []string{"//visibility:public"})
			rules = append(rules, r)

			var refs []includeRef
			for _, header := range group {
				dir := path.Join(args.Rel, path.Dir(header))
				for _, inc := range includes[header] {
					refs = append(refs, includeRef{dir: dir, inc: inc})
				}
			}
			imports = append(imports, refs)
		}
	}

	// Programs keep their hand-written attributes; only deps is re-derived from
	// the includes of their source.
	if args.File != nil {
		for _, existing := range args.File.Rules {
			if !isProgKind(existing.Kind()) {
				continue
			}
			src := existing.AttrString("src")
			if src == "" {
				continue
			}
			dir, absolute := srcLocation(args.Config.RepoRoot, args.Rel, src)
			r := rule.NewRule(existing.Kind(), existing.Name())
			r.SetAttr("src", src)
			rules = append(rules, r)

			var refs []includeRef
			for _, inc := range quotedIncludes(absolute) {
				refs = append(refs, includeRef{dir: dir, inc: inc})
			}
			imports = append(imports, refs)
		}
	}

	return language.GenerateResult{
		Gen:     rules,
		Imports: imports,
		Empty:   retracted(args.File, live),
	}
}

func isProgKind(kind string) bool {
	for _, known := range progKinds {
		if kind == known {
			return true
		}
	}
	return false
}

// retracted lists generated header libraries whose headers are gone, without
// touching hand-written cc_library targets that never came from a header.
func retracted(f *rule.File, live map[string]bool) []*rule.Rule {
	if f == nil {
		return nil
	}
	var empty []*rule.Rule
	for _, r := range f.Rules {
		if r.Kind() != hdrKind || live[r.Name()] {
			continue
		}
		if hdrs := r.AttrStrings("hdrs"); len(hdrs) > 0 && hdrPrefix+strings.TrimSuffix(hdrs[0], ".h") == r.Name() {
			empty = append(empty, rule.NewRule(hdrKind, r.Name()))
		}
	}
	return empty
}

func (*lang) Imports(_ *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	if r.Kind() != hdrKind {
		return nil
	}
	// Indexed under the path an #include would spell, which with includes = ["."]
	// is the header path relative to the package, and under the repo-relative
	// path so a same-tree include wins over an identically named header
	// in another tree.
	var specs []resolve.ImportSpec
	for _, hdr := range r.AttrStrings("hdrs") {
		specs = append(specs,
			resolve.ImportSpec{Lang: langName, Imp: hdr},
			resolve.ImportSpec{Lang: langName, Imp: path.Join(f.Pkg, hdr)},
		)
	}
	return specs
}

func (*lang) Resolve(c *config.Config, ix *resolve.RuleIndex, _ *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	refs, ok := imports.([]includeRef)
	if !ok {
		return
	}
	seen := map[string]bool{}
	deps := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ruleOwnedIncludes[ref.inc] {
			continue
		}
		dep, found := resolveInclude(c, ix, from, ref)
		if !found || seen[dep] {
			continue
		}
		seen[dep] = true
		deps = append(deps, dep)
	}
	if len(deps) == 0 {
		r.DelAttr("deps")
		return
	}
	sort.Strings(deps)
	r.SetAttr("deps", deps)
}

func resolveInclude(c *config.Config, ix *resolve.RuleIndex, from label.Label, ref includeRef) (string, bool) {
	candidates := []string{path.Join(ref.dir, ref.inc), ref.inc}
	// Overrides are consulted for every spelling before the index, so a
	// # gazelle:resolve directive is not shadowed by an indexed header.
	for _, imp := range candidates {
		spec := resolve.ImportSpec{Lang: langName, Imp: imp}
		if override, ok := resolve.FindRuleWithOverride(c, spec, langName); ok {
			return override.Rel(from.Repo, from.Pkg).String(), true
		}
	}
	for _, imp := range candidates {
		spec := resolve.ImportSpec{Lang: langName, Imp: imp}
		if best, ok := nearest(ix.FindRulesByImportWithConfig(c, spec, langName), from, ref.dir); ok {
			return best.Rel(from.Repo, from.Pkg).String(), true
		}
	}
	// Kernel and toolchain headers are not indexed; they come from the
	// linux_headers repositories and carry no cc_library edge.
	return "", false
}

// nearest picks the match whose package encloses the including file, so that a
// bare include like "types.h" binds to the copy in the includer's own tree
// rather than to a same-named header elsewhere in the repository.
func nearest(matches []resolve.FindResult, from label.Label, dir string) (label.Label, bool) {
	var best label.Label
	found, bestLen := false, -1
	var fallbacks []label.Label
	for _, m := range matches {
		if m.IsSelfImport(from) {
			continue
		}
		pkg := m.Label.Pkg
		if dir == pkg || strings.HasPrefix(dir, pkg+"/") {
			if len(pkg) > bestLen {
				best, bestLen, found = m.Label, len(pkg), true
			}
			continue
		}
		fallbacks = append(fallbacks, m.Label)
	}
	if found {
		return best, true
	}
	if len(fallbacks) == 0 {
		return label.NoLabel, false
	}
	sort.Slice(fallbacks, func(i, j int) bool { return fallbacks[i].String() < fallbacks[j].String() })
	return fallbacks[0], true
}
