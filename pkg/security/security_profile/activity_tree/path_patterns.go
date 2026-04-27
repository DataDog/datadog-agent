// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// PathPatternConfig controls sibling pattern mining on FileNode maps.
//
// The activity tree is already a prefix tree keyed by path component, so
// pattern mining only needs to act on the *clustering* step: sibling
// children of a FileNode (or ProcessNode.Files) that share a structural
// signature are collapsed into a single pattern node whose Name carries a
// wildcard template.
//
// Pattern mining is opt-in per ActivityTree (via Stats.SetPathPatternConfig).
// v2 security profiles enable it explicitly; v1 profiles and activity
// dumps leave it off so their runtime behavior is unchanged.
type PathPatternConfig struct {
	// Enabled turns pattern mining on for this tree. When false, every
	// gate short-circuits and the tree behaves exactly like it did
	// before path-pattern mining was introduced.
	Enabled bool

	// MaxChildren is the fan-out threshold above which a merge pass runs
	// on insertion. Zero disables threshold-based merging (the finalize
	// pass can still merge).
	MaxChildren int

	// MinClusterSize is the minimum number of same-structure siblings
	// required to collapse them into a single pattern node during an
	// insertion-time merge pass.
	MinClusterSize int

	// MinClusterSizeOnFinalize is the equivalent threshold applied by
	// FinalizePatterns, typically lower than MinClusterSize so short-lived
	// profiles still benefit from patterning before being persisted.
	MinClusterSizeOnFinalize int
}

// DefaultPathPatternConfig returns the default enabled configuration used
// by callers that want pattern mining on (today: v2 security profiles).
// The defaults balance the runtime cost of mining (bounded by MaxChildren)
// with the compression benefit (MinClusterSize on fan-out, smaller
// MinClusterSizeOnFinalize for the persist-time pass).
func DefaultPathPatternConfig() PathPatternConfig {
	return PathPatternConfig{
		Enabled:                  true,
		MaxChildren:              15,
		MinClusterSize:           5,
		MinClusterSizeOnFinalize: 3,
	}
}

// patternWildcard is the character used in merged FileNode names to stand
// in for variable sub-tokens. It matches any non-empty run of characters
// inside a single path component.
const patternWildcard = "*"

// ---------------------------------------------------------------------------
// Structural signature
// ---------------------------------------------------------------------------

// structureSignature builds a canonical signature describing the skeleton
// of a path component: separator positions and per-sub-token class.
// Siblings with the same signature share a skeleton and are candidates for
// merging into a single template.
//
// Sub-tokens are delimited by '-', '.' or '_'. Each sub-token is classified
// as:
//   - "N" if it contains only digits
//   - "A" if it contains only alphabetic characters
//   - "M" if it contains a mix of alphanumeric characters
//   - the literal sub-token otherwise (rare separators inside the name)
//
// Examples:
//
//	"sess-abc"         -> "A-A"
//	"sess-aaa"         -> "A-A"
//	"sess-42"          -> "A-N"
//	"pod-abc123-xyz"   -> "A-M-A"
//	"2024-01-15.log"   -> "N-N-N.A"
//	"1337"             -> "N"
//	"config.json"      -> "A.A"
func structureSignature(name string) string {
	if name == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(len(name))
	i := 0
	for i < len(name) {
		if isSeparator(name[i]) {
			out.WriteByte(name[i])
			i++
			continue
		}
		j := i
		for j < len(name) && !isSeparator(name[j]) {
			j++
		}
		out.WriteString(classifySubToken(name[i:j]))
		i = j
	}
	return out.String()
}

// classifySubToken returns the class letter of a separator-free sub-token.
func classifySubToken(sub string) string {
	if sub == "" {
		return ""
	}
	hasAlpha := false
	hasDigit := false
	for i := 0; i < len(sub); i++ {
		c := sub[i]
		switch {
		case isDigit(c):
			hasDigit = true
		case isAlpha(c):
			hasAlpha = true
		default:
			// Sub-token contains an unexpected character (e.g. ':', '@').
			// Preserve it literally to avoid over-merging weird names.
			return sub
		}
	}
	switch {
	case hasAlpha && hasDigit:
		return "M"
	case hasDigit:
		return "N"
	case hasAlpha:
		return "A"
	}
	return sub
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isAlpha(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isSeparator(c byte) bool {
	return c == '-' || c == '.' || c == '_'
}

// isBareWildcardTemplate reports whether a merge template contains no
// literal character — only wildcards and (optionally) separators. Such
// a template leaves no anchor tying it to its members' original names
// and is therefore only safe to emit when the bucket's signature
// indicates variable content (numeric / mixed).
func isBareWildcardTemplate(template string) bool {
	if template == "" {
		return false
	}
	hasWildcard := false
	for i := 0; i < len(template); i++ {
		c := template[i]
		switch {
		case c == patternWildcard[0]:
			hasWildcard = true
		case isSeparator(c):
			// separators are skeleton, not literal content
		default:
			return false
		}
	}
	return hasWildcard
}

// signatureHasVariableClass reports whether a structural signature has
// at least one sub-token classified as variable — "N" (numeric) or
// "M" (mixed alphanumeric). Signatures made exclusively of "A" tokens
// and separators describe fixed alpha names whose content carries the
// identity; they must not be collapsed into a bare-wildcard template.
func signatureHasVariableClass(sig string) bool {
	return strings.ContainsAny(sig, "NM")
}

// ---------------------------------------------------------------------------
// Template building & matching
// ---------------------------------------------------------------------------

// buildTemplate returns the merged Name for a cluster of same-signature
// siblings. For each sub-token position, if every sibling has the same
// literal content the literal is kept; otherwise it is replaced by the
// wildcard. Separator positions are inherited from the shared signature
// (all siblings in the cluster have identical separator layout).
//
// Example: [sess-aaa, sess-bbb, sess-ccc] -> "sess-*"
// Example: [2024-01-15.log, 2024-01-16.log] -> "2024-01-*.log"
func buildTemplate(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	// All members share the same separator layout. Use the first name to
	// drive the walk; read literals from each member in lockstep.
	var (
		out   strings.Builder
		heads = make([]int, len(names))
	)
	ref := names[0]
	out.Grow(len(ref))
	i := 0
	for i < len(ref) {
		if isSeparator(ref[i]) {
			out.WriteByte(ref[i])
			for k := range heads {
				heads[k]++
			}
			i++
			continue
		}
		// walk the current sub-token on the reference name
		j := i
		for j < len(ref) && !isSeparator(ref[j]) {
			j++
		}
		refSub := ref[i:j]
		allSame := true
		// compare with the sub-token at the same position in every other name
		for k := 1; k < len(names); k++ {
			name := names[k]
			start := heads[k]
			end := start
			for end < len(name) && !isSeparator(name[end]) {
				end++
			}
			if name[start:end] != refSub {
				allSame = false
			}
			heads[k] = end
		}
		if allSame {
			out.WriteString(refSub)
		} else {
			out.WriteString(patternWildcard)
		}
		heads[0] = j
		i = j
	}
	return out.String()
}

// templateMatches reports whether name conforms to the given template.
// The wildcard "*" in template stands for one or more characters (no slash,
// which is guaranteed by the caller since this is a single path component).
func templateMatches(template, name string) bool {
	if !strings.Contains(template, patternWildcard) {
		return template == name
	}
	parts := strings.Split(template, patternWildcard)
	// Leading literal must prefix the name.
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	pos := len(parts[0])
	// Middle literals: require each to appear in order, each wildcard
	// consuming at least one character.
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			// Adjacent wildcards collapse to one wildcard; the next literal
			// handles the consumption check.
			continue
		}
		remaining := name[pos:]
		idx := strings.Index(remaining, parts[i])
		if idx < 1 {
			return false
		}
		pos += idx + len(parts[i])
	}
	// Trailing literal suffix check, with at least one char consumed by the
	// last wildcard.
	last := parts[len(parts)-1]
	if last == "" {
		// template ends with "*": any non-empty tail satisfies it.
		return len(name) > pos
	}
	if !strings.HasSuffix(name, last) {
		return false
	}
	return len(name) >= pos+len(last)+1
}

// ---------------------------------------------------------------------------
// Sibling grouping & merging
// ---------------------------------------------------------------------------

type signatureBucket struct {
	signature string
	members   []string
}

// groupChildrenBySignature partitions children by their structure
// signature. Existing pattern nodes (IsPattern == true) are excluded from
// bucketing — a pattern node is already the result of a previous merge and
// must not be re-merged with literal siblings. The result is sorted by
// signature for deterministic iteration in tests.
func groupChildrenBySignature(children map[string]*FileNode) []signatureBucket {
	byKey := make(map[string][]string)
	for name, child := range children {
		if child == nil || child.IsPattern {
			continue
		}
		sig := structureSignature(name)
		byKey[sig] = append(byKey[sig], name)
	}
	out := make([]signatureBucket, 0, len(byKey))
	for sig, names := range byKey {
		sort.Strings(names)
		out = append(out, signatureBucket{signature: sig, members: names})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].signature < out[j].signature })
	return out
}

// mergeFileNodes folds src into dst in place: unions Children, Seen,
// MatchedRules, Open flags/mode, and the earliest GenerationType. dst's
// Name is preserved; callers replace it with the cluster template.
func (dst *FileNode) mergeInto(src *FileNode) {
	if src == nil {
		return
	}
	// Image tag timestamps: union, preserving earliest FirstSeen and latest LastSeen.
	for tag, times := range src.Seen {
		if times == nil {
			continue
		}
		if existing, ok := dst.Seen[tag]; ok && existing != nil {
			if times.FirstSeen.Before(existing.FirstSeen) {
				existing.FirstSeen = times.FirstSeen
			}
			if times.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = times.LastSeen
			}
		} else {
			cp := *times
			dst.Seen[tag] = &cp
		}
	}

	dst.MatchedRules = model.AppendMatchedRule(dst.MatchedRules, src.MatchedRules)

	// Open: union flags/mode. Keep the first non-nil SyscallEvent as the
	// representative for the syscall context.
	if src.Open != nil {
		if dst.Open == nil {
			cp := *src.Open
			dst.Open = &cp
		} else {
			dst.Open.Flags |= src.Open.Flags
			dst.Open.Mode |= src.Open.Mode
		}
	}

	// GenerationType: prefer the more "authoritative" type, in priority
	// order Snapshot > Runtime > WorkloadWarmup > ProfileDrift > Unknown.
	if generationPriority(src.GenerationType) > generationPriority(dst.GenerationType) {
		dst.GenerationType = src.GenerationType
	}

	// File: keep dst's representative; callers have already rewritten its
	// BasenameStr to the cluster template.

	// Recursive merge of the children.
	for name, child := range src.Children {
		if existing, ok := dst.Children[name]; ok {
			existing.mergeInto(child)
		} else {
			dst.Children[name] = child
		}
	}
}

func generationPriority(t NodeGenerationType) int {
	switch t {
	case Snapshot:
		return 4
	case Runtime:
		return 3
	case WorkloadWarmup:
		return 2
	case ProfileDrift:
		return 1
	}
	return 0
}

// collapseBucket folds the given cluster of siblings into a single merged
// FileNode with the provided template as its Name, and installs it in
// children. The members are removed from children. Returns true if a merge
// was actually performed (i.e. the bucket had more than one member).
func collapseBucket(children map[string]*FileNode, template string, members []string, stats *Stats) bool {
	if len(members) < 2 {
		return false
	}
	head := children[members[0]]
	if head == nil {
		return false
	}
	// Promote head to a pattern node.
	head.Name = template
	head.IsPattern = true
	if head.File != nil {
		head.File.BasenameStr = template
	}
	for _, name := range members[1:] {
		sibling := children[name]
		if sibling == nil {
			continue
		}
		head.mergeInto(sibling)
		delete(children, name)
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	}
	delete(children, members[0])
	// Reinstall under the (possibly already existing) template key: if a
	// pattern node with the same template already exists, fold into it.
	if existing, ok := children[template]; ok && existing != head {
		existing.mergeInto(head)
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	} else {
		children[template] = head
	}
	if stats != nil {
		stats.FilePatternsCreated++
	}
	return true
}

// mergeChildren runs one path-pattern merge pass over the map, collapsing
// every signature bucket whose size is ≥ minClusterSize into a single
// pattern node. Pattern nodes already present in children are left alone.
// Returns the number of buckets that were collapsed.
func mergeChildren(children map[string]*FileNode, minClusterSize int, stats *Stats) int {
	if len(children) == 0 || minClusterSize < 2 {
		return 0
	}
	buckets := groupChildrenBySignature(children)
	collapsed := 0
	for _, b := range buckets {
		if len(b.members) < minClusterSize {
			continue
		}
		template := buildTemplate(b.members)
		if template == "" {
			continue
		}
		// Skip buckets where the template has no wildcard — the members
		// were already identical, which means we'd have seen only one map
		// entry. Defensive: guarantees we never "merge" identical names.
		if !strings.Contains(template, patternWildcard) {
			continue
		}
		// A template that is only wildcards (and separators) carries no
		// literal anchor — the members share just a structural shape.
		// That's fine for enumerated identifiers (numeric, hex, mixed
		// alphanumeric), but merging distinct fixed alpha names like
		// "tmp"/"var"/"etc" would fold unrelated top-level directories
		// into a single "*" node and lose their identity. Require at
		// least one variable-class sub-token in the signature before
		// accepting a bare-wildcard merge.
		if isBareWildcardTemplate(template) && !signatureHasVariableClass(b.signature) {
			continue
		}
		if collapseBucket(children, template, b.members, stats) {
			collapsed++
		}
	}
	return collapsed
}

// maybeMergeChildren is called after an insertion adds a new child to
// children. It runs a merge pass only if pattern mining is enabled on
// this tree (via stats.patternCfg) AND the child count exceeds the
// configured MaxChildren fan-out threshold, bounding the amortized cost
// of pattern mining.
func maybeMergeChildren(children map[string]*FileNode, stats *Stats) int {
	cfg := pathPatternCfgFrom(stats)
	if !cfg.Enabled || cfg.MaxChildren <= 0 || len(children) <= cfg.MaxChildren {
		return 0
	}
	return mergeChildren(children, cfg.MinClusterSize, stats)
}

// findChildWithPatternFallback looks up an exact child by name, and if
// that misses falls back to any sibling pattern node whose template
// matches the input name. This is what makes anomaly detection quiet on
// pattern variants: the dry-run insertion walk hits an existing pattern
// instead of flagging the event as a new entry.
//
// When pattern mining is disabled on the tree (stats.patternCfg.Enabled
// is false), no pattern nodes can exist and the loop would be pure
// overhead. Gate on the config so trees that never opt in pay only the
// single map lookup they used to.
func findChildWithPatternFallback(children map[string]*FileNode, name string, stats *Stats) (*FileNode, bool) {
	if c, ok := children[name]; ok {
		return c, true
	}
	if !pathPatternCfgFrom(stats).Enabled {
		return nil, false
	}
	for _, c := range children {
		if c == nil || !c.IsPattern {
			continue
		}
		if templateMatches(c.Name, name) {
			return c, true
		}
	}
	return nil, false
}

// pathPatternCfgFrom returns the PathPatternConfig associated with the
// stats (i.e. with the owning ActivityTree). Nil stats → zero config
// (disabled), which keeps unit tests that construct bare Stats values
// safe by default.
func pathPatternCfgFrom(stats *Stats) PathPatternConfig {
	if stats == nil {
		return PathPatternConfig{}
	}
	return stats.patternCfg
}
