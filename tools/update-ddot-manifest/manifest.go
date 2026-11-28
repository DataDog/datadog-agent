// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// distVersionKey is the synthetic change label used for the dist.version field.
const distVersionKey = "dist.version"

// datadogPrefix marks first-party modules: they are known-but-untouched (never
// reported as custom). Components to bump are identified by the components table,
// not by a prefix; anything neither in that table nor under datadogPrefix is a
// custom component.
const datadogPrefix = "github.com/DataDog/"

// componentCategories are the manifest top-level keys whose entries carry a
// "gomod: <module> <version>" scalar.
var componentCategories = []string{
	"receivers",
	"processors",
	"exporters",
	"connectors",
	"extensions",
	"providers",
}

// change records a single version rewrite for reporting.
type change struct {
	module string
	oldV   string
	newV   string
	line   int // yaml.Node.Line (1-indexed)
	column int // yaml.Node.Column (0-indexed)
}

// updateResult summarizes what updateManifest did.
type updateResult struct {
	changed []change // version rewrites that were applied
	custom  []string // custom module paths detected and left untouched
}

// updateManifest rewrites the version of every known (core/contrib) component
// in an OCB manifest according to v, updates dist.version, and reports any
// custom component left untouched.
//
// The change set is computed by parsing the YAML (robust against the folded
// "gomod" scalars), but the changes are applied as targeted text edits on the
// original bytes so that formatting, comments and the replaces section are
// preserved byte-for-byte — only version tokens change.
func updateManifest(data []byte, v ddotVersion) ([]byte, *updateResult, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil, fmt.Errorf("manifest is not a valid YAML document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("manifest root is not a mapping")
	}

	res := &updateResult{}

	// dist.version -> OCBVersion.
	if dist := mappingValue(root, "dist"); dist != nil {
		if ver := mappingValue(dist, "version"); ver != nil && ver.Kind == yaml.ScalarNode {
			if ver.Value != v.OCBVersion {
				res.changed = append(res.changed, change{
					module: distVersionKey,
					oldV:   ver.Value,
					newV:   v.OCBVersion,
					line:   ver.Line,
					column: ver.Column,
				})
			}
		}
	}

	// Each component category.
	for _, cat := range componentCategories {
		seq := mappingValue(root, cat)
		if seq == nil || seq.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range seq.Content {
			gomod := mappingValue(item, "gomod")
			if gomod == nil || gomod.Kind != yaml.ScalarNode {
				continue
			}
			// The value is "<module> <version>"; anything else is left as-is.
			fields := strings.Fields(gomod.Value)
			if len(fields) != 2 {
				continue
			}
			module, curVer := fields[0], fields[1]

			newVer, known := newVersionFor(module, v)
			if !known {
				if !strings.HasPrefix(module, datadogPrefix) {
					res.custom = append(res.custom, module)
				}
				continue
			}
			if newVer != curVer {
				res.changed = append(res.changed, change{
					module: module,
					oldV:   curVer,
					newV:   newVer,
					line:   gomod.Line,
					column: gomod.Column,
				})
			}
		}
	}

	out, err := applyChanges(data, res.changed)
	if err != nil {
		return nil, nil, err
	}
	return out, res, nil
}

// applyChanges rewrites each version token in the raw manifest bytes using the
// exact byte positions recorded by the YAML parser. For each change it jumps to
// the node's offset, then searches forward for oldV from that position — which
// naturally handles quoted scalars (e.g. version: "0.159.0") without any special
// casing. Replacements are applied back-to-front so that earlier offsets are
// never invalidated by a preceding edit.
func applyChanges(data []byte, changes []change) ([]byte, error) {
	type replacement struct {
		start, end int
		newV       string
		module     string
	}

	reps := make([]replacement, 0, len(changes))
	for _, c := range changes {
		offset, err := lineColToOffset(data, c.line, c.column)
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", c.module, err)
		}
		idx := bytes.Index(data[offset:], []byte(c.oldV))
		if idx == -1 {
			return nil, fmt.Errorf("module %q: could not find %q at line %d", c.module, c.oldV, c.line)
		}
		start := offset + idx
		reps = append(reps, replacement{
			start:  start,
			end:    start + len(c.oldV),
			newV:   c.newV,
			module: c.module,
		})
	}

	// Apply back-to-front so earlier offsets stay valid.
	sort.Slice(reps, func(i, j int) bool { return reps[i].start > reps[j].start })

	out := make([]byte, len(data))
	copy(out, data)
	for _, r := range reps {
		out = append(out[:r.start], append([]byte(r.newV), out[r.end:]...)...)
	}
	return out, nil
}

// newVersionFor returns the target version for a component and whether it is a
// known component that should be bumped. Modules absent from the components
// catalog (Datadog and third-party) return known=false and are left untouched by
// the caller.
//
// The target is picked from the four version numbers of the release by the
// component's stream (core/contrib) and its stability at that release: the stable
// (v1.x) version when the module has a FirstStable and the release's stable
// version is at least that, the unstable (v0.x) version otherwise — including
// when a release older than FirstStable is requested.
func newVersionFor(module string, v ddotVersion) (string, bool) {
	c, ok := components[module]
	if !ok {
		return "", false
	}

	var stableVer, unstableVer string
	switch c.Stream {
	case streamCore:
		stableVer, unstableVer = v.CoreStableVersion, v.CoreUnstableVersion
	case streamContrib:
		stableVer, unstableVer = v.ContribStableVersion, v.ContribUnstableVersion
	default:
		return "", false
	}

	if c.FirstStable != "" && semverGTE(stableVer, c.FirstStable) {
		return stableVer, true
	}
	return unstableVer, true
}

// semverGTE reports whether version a is greater than or equal to version b.
// Both are collector version tags of the form "vMAJOR.MINOR.PATCH" (e.g.
// "v1.65.0"). The comparison is numeric, field by field — a lexical string
// compare would wrongly rank "v1.9.0" above "v1.10.0".
//
// Inputs come from the hardcoded tables and are expected to be well formed; any
// missing or non-numeric field parses as 0 rather than failing.
func semverGTE(a, b string) bool {
	va, vb := parseSemver(a), parseSemver(b)
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return true // all fields equal -> a == b, which satisfies >=
}

// parseSemver splits a "vMAJOR.MINOR.PATCH" tag into its three numeric fields.
// A leading "v" is optional; missing or non-numeric fields default to 0.
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	var out [3]int
	for i, field := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(field)
	}
	return out
}

// lineColToOffset converts a 1-indexed (line, col) position from a yaml.Node
// into a 0-indexed byte offset into data.
func lineColToOffset(data []byte, line, col int) (int, error) {
	cur := 1
	lineStart := 0
	for i, b := range data {
		if cur == line {
			return lineStart + col - 1, nil
		}
		if b == '\n' {
			cur++
			lineStart = i + 1
		}
	}
	if cur == line {
		return lineStart + col - 1, nil
	}
	return 0, fmt.Errorf("line %d out of range (file has %d lines)", line, cur)
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
