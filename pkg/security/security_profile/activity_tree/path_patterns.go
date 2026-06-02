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
// Mining is opt-in per ActivityTree via Stats.SetPathPatternConfig; v2
// security profiles enable it, v1 profiles and activity dumps leave it
// off.
type PathPatternConfig struct {
	Enabled                  bool
	MaxChildren              int
	MinClusterSize           int
	MinClusterSizeOnFinalize int
}

// DefaultPathPatternConfig returns the enabled configuration used by v2
// security profiles.
func DefaultPathPatternConfig() PathPatternConfig {
	return PathPatternConfig{
		Enabled:                  true,
		MaxChildren:              15,
		MinClusterSize:           5,
		MinClusterSizeOnFinalize: 3,
	}
}

const patternWildcard = "*"

// structureSignature returns a canonical skeleton of name: separator
// positions and per-sub-token class ("N" digits, "A" alpha, "M" mixed).
// Examples: "sess-42" -> "A-N", "2024-01-15.log" -> "N-N-N.A".
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
			// preserve unexpected characters literally to avoid over-merging
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

// isBareWildcardTemplate reports whether template contains only
// wildcards and separators (no literal anchor).
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
		default:
			return false
		}
	}
	return hasWildcard
}

// signatureHasVariableClass reports whether sig contains an "N" or "M"
// sub-token class. Pure-alpha signatures describe fixed names and must
// not be collapsed into a bare wildcard.
func signatureHasVariableClass(sig string) bool {
	return strings.ContainsAny(sig, "NM")
}

// buildTemplate returns the merged name for a cluster of same-signature
// siblings, keeping per-position literals where all siblings agree and
// using the wildcard otherwise.
// Example: [sess-aaa, sess-bbb] -> "sess-*".
func buildTemplate(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	// All members share the same separator layout; use names[0] as the
	// reference and read sub-tokens from the rest in lockstep.
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
		j := i
		for j < len(ref) && !isSeparator(ref[j]) {
			j++
		}
		refSub := ref[i:j]
		allSame := true
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

// templateMatches reports whether name conforms to template. Each "*"
// stands for one or more characters within a single path component.
func templateMatches(template, name string) bool {
	if !strings.Contains(template, patternWildcard) {
		return template == name
	}
	parts := strings.Split(template, patternWildcard)
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	pos := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}
		remaining := name[pos:]
		idx := strings.Index(remaining, parts[i])
		if idx < 1 {
			return false
		}
		pos += idx + len(parts[i])
	}
	last := parts[len(parts)-1]
	if last == "" {
		return len(name) > pos
	}
	if !strings.HasSuffix(name, last) {
		return false
	}
	return len(name) >= pos+len(last)+1
}

type signatureBucket struct {
	signature string
	members   []string
}

// groupChildrenBySignature partitions non-pattern children by structural
// signature, sorted by signature for deterministic iteration.
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

// mergeInto folds src into dst in place: unions Children, Seen,
// MatchedRules, Open flags/mode, and keeps the more authoritative
// GenerationType. dst.Name is left to the caller.
func (dst *FileNode) mergeInto(src *FileNode) {
	if src == nil {
		return
	}
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

	if src.Open != nil {
		if dst.Open == nil {
			cp := *src.Open
			dst.Open = &cp
		} else {
			dst.Open.Flags |= src.Open.Flags
			dst.Open.Mode |= src.Open.Mode
		}
	}

	if generationPriority(src.GenerationType) > generationPriority(dst.GenerationType) {
		dst.GenerationType = src.GenerationType
	}

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

// collapseBucket folds members into a single pattern node named template
// and installs it in children. bucketSignature is stored on the resulting
// node for the anomaly-time signature check.
func collapseBucket(children map[string]*FileNode, template string, members []string, bucketSignature string, stats *Stats) bool {
	if len(members) < 2 {
		return false
	}
	head := children[members[0]]
	if head == nil {
		return false
	}
	head.Name = template
	head.IsPattern = true
	head.PatternSignature = bucketSignature
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
	if existing, ok := children[template]; ok && existing != head {
		existing.mergeInto(head)
		if existing.PatternSignature == "" {
			existing.PatternSignature = bucketSignature
		}
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	} else {
		children[template] = head
	}
	return true
}

// mergeChildren runs one merge pass over children, collapsing every
// signature bucket with at least minClusterSize members into a pattern
// node. Returns the number of buckets collapsed.
//
// Bare-wildcard templates (no literal anchor) have two extra gates:
//   - the bucket signature must contain a variable class, otherwise we
//     would fold distinct fixed alpha names (tmp / var / etc) into "*";
//   - the parent must hold a single non-pattern signature, otherwise a
//     bare "*" would silently absorb future variants of a coexisting
//     shape — including anomalies.
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
		if template == "" || !strings.Contains(template, patternWildcard) {
			continue
		}
		if isBareWildcardTemplate(template) {
			if !signatureHasVariableClass(b.signature) {
				continue
			}
			if len(buckets) != 1 {
				continue
			}
		}
		if collapseBucket(children, template, b.members, b.signature, stats) {
			collapsed++
		}
	}
	return collapsed
}

// maybeMergeChildren runs a merge pass when mining is enabled and the
// child count exceeds the configured fan-out threshold.
func maybeMergeChildren(children map[string]*FileNode, stats *Stats) int {
	cfg := pathPatternCfgFrom(stats)
	if !cfg.Enabled || cfg.MaxChildren <= 0 || len(children) <= cfg.MaxChildren {
		return 0
	}
	return mergeChildren(children, cfg.MinClusterSize, stats)
}

// findChildWithPatternFallback returns the exact-name child if present,
// otherwise a sibling pattern node whose template matches name. When a
// pattern carries a non-empty PatternSignature, name must share it. A
// disabled config skips the wildcard scan entirely.
func findChildWithPatternFallback(children map[string]*FileNode, name string, stats *Stats) (*FileNode, bool) {
	if c, ok := children[name]; ok {
		return c, true
	}
	if !pathPatternCfgFrom(stats).Enabled {
		return nil, false
	}
	var (
		nameSig  string
		sigReady bool
	)
	for _, c := range children {
		if c == nil || !c.IsPattern {
			continue
		}
		if !templateMatches(c.Name, name) {
			continue
		}
		if c.PatternSignature != "" {
			if !sigReady {
				nameSig = structureSignature(name)
				sigReady = true
			}
			if nameSig != c.PatternSignature {
				continue
			}
		}
		return c, true
	}
	return nil, false
}

func pathPatternCfgFrom(stats *Stats) PathPatternConfig {
	if stats == nil {
		return PathPatternConfig{}
	}
	return stats.patternCfg
}
