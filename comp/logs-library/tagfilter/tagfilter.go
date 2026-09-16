// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagfilter removes user-configured log tags before intake encoding.
// Matching uses ASCII-case-insensitive key:value patterns with value-only wildcards.
package tagfilter

import (
	"fmt"
	"strings"
)

// RejectedPattern is a pattern that failed to compile, with an actionable reason.
type RejectedPattern struct {
	Pattern string
	Reason  string
}

// Report describes what Compile did with the patterns it was given.
type Report struct {
	Rejected []RejectedPattern
	Warnings []string
}

// Patterns is the compiled pattern set, for the status page.
type Patterns struct {
	Include []string
	Exclude []string
}

// IsEmpty reports whether the set holds no patterns.
func (p Patterns) IsEmpty() bool {
	return len(p.Include) == 0 && len(p.Exclude) == 0
}

// Filters is a compiled set of include and exclude patterns for one scope.
type Filters struct {
	byKey    map[string]*keyRules
	patterns Patterns
}

// keyRules holds every rule that applies to one literal tag key.
type keyRules struct {
	includeAny  bool
	includeVals []valueGlob
	excludeAny  bool
	excludeVals []valueGlob
}

// valueGlob matches a tag value split on "*" at compile time.
type valueGlob struct {
	segments []string
}

// Compile builds a filter, dropping and reporting invalid patterns instead of
// failing the full configuration.
func Compile(include, exclude []string) (*Filters, Report) {
	byKey := make(map[string]*keyRules)

	var report Report
	f := &Filters{byKey: byKey}
	f.patterns = Patterns{
		Include: compileList(byKey, include, false, &report),
		Exclude: compileList(byKey, exclude, true, &report),
	}
	return f, report
}

// compileList validates and compiles one pattern list, returning the patterns
// (trimmed, deduplicated) that survived.
func compileList(byKey map[string]*keyRules, patterns []string, isExclude bool, report *Report) []string {
	seen := make(map[string]struct{}, len(patterns))
	kept := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		key, value, reason := parsePattern(p)
		dedupKey := p
		if reason == "" {
			dedupKey = asciiLower(p)
		}
		if _, dup := seen[dedupKey]; dup {
			continue
		}
		seen[dedupKey] = struct{}{}

		if reason != "" {
			report.Rejected = append(report.Rejected, RejectedPattern{Pattern: p, Reason: reason})
			continue
		}

		folded := asciiLower(key)
		if isExclude && isPayloadAttributeKey(folded) {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"exclude pattern %q targets %q, which remains present as a log attribute",
				p, folded))
		}

		kr := byKey[folded]
		if kr == nil {
			kr = &keyRules{}
			byKey[folded] = kr
		}
		if value == "*" {
			if isExclude {
				kr.excludeAny = true
			} else {
				kr.includeAny = true
			}
		} else {
			g := valueGlob{segments: strings.Split(asciiLower(value), "*")}
			if isExclude {
				kr.excludeVals = append(kr.excludeVals, g)
			} else {
				kr.includeVals = append(kr.includeVals, g)
			}
		}
		kept = append(kept, p)
	}
	return kept
}

// parsePattern splits and validates a single trimmed pattern, returning a
// non-empty reason if it is rejected.
func parsePattern(p string) (key, value, reason string) {
	if p == "" {
		return "", "", fmt.Sprintf("pattern %q is empty", p)
	}
	if p == "*" || p == "*:*" {
		return "", "", fmt.Sprintf("pattern %q would match every tag and is not allowed", p)
	}
	idx := strings.IndexByte(p, ':')
	if idx < 0 {
		return "", "", fmt.Sprintf("pattern %q must use key:value syntax", p)
	}
	key, value = p[:idx], p[idx+1:]
	if key == "" {
		return "", "", fmt.Sprintf("pattern %q has an empty key", p)
	}
	if strings.ContainsRune(key, '*') {
		return "", "", fmt.Sprintf(
			"pattern %q has \"*\" in the key; wildcards are only allowed in the value", p)
	}
	return key, value, ""
}

func isPayloadAttributeKey(key string) bool {
	return key == "source" || key == "service" || key == "host"
}

// asciiLower lowercases ASCII letters in s and returns s unchanged when possible.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if c := b[j]; c >= 'A' && c <= 'Z' {
					b[j] = c + 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

func foldByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		c += 'a' - 'A'
	}
	return c
}

// equalFolded reports whether s ASCII-case-folds to folded.
func equalFolded(folded, s string) bool {
	if len(folded) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if foldByte(s[i]) != folded[i] {
			return false
		}
	}
	return true
}

// splitTag splits a tag on its first colon. ok is false when tag carries no
// colon, meaning it can never be matched by any pattern.
func splitTag(tag string) (key, value string, ok bool) {
	idx := strings.IndexByte(tag, ':')
	if idx < 0 {
		return "", "", false
	}
	return tag[:idx], tag[idx+1:], true
}

type decision int

const (
	noDecision decision = iota
	keepDecision
	dropDecision
)

// decide reports how f's own rules apply to key:value, independent of any other
// scope. noDecision means f has no opinion.
func (f *Filters) decide(key, value string) decision {
	if f == nil {
		return noDecision
	}
	kr := f.byKey[asciiLower(key)]
	if kr == nil {
		return noDecision
	}
	if kr.includeAny || matchesGlob(kr.includeVals, value) {
		return keepDecision
	}
	if kr.excludeAny || matchesGlob(kr.excludeVals, value) {
		return dropDecision
	}
	return noDecision
}

func matchesGlob(globs []valueGlob, value string) bool {
	for _, g := range globs {
		if g.match(value) {
			return true
		}
	}
	return false
}

func (g valueGlob) match(value string) bool {
	segs := g.segments
	if len(segs) == 1 {
		return equalFolded(segs[0], value)
	}
	head, tail := segs[0], segs[len(segs)-1]
	if !hasPrefixFolded(value, head) {
		return false
	}
	// Consume the head before matching the tail so overlapping runs (pattern
	// "a*a" against value "a") can't have one character satisfy both ends.
	value = value[len(head):]
	if !hasSuffixFolded(value, tail) {
		return false
	}
	value = value[:len(value)-len(tail)]
	for _, mid := range segs[1 : len(segs)-1] {
		i := indexFolded(value, mid)
		if i < 0 {
			return false
		}
		value = value[i+len(mid):]
	}
	return true
}

func hasPrefixFolded(s, foldedPrefix string) bool {
	return len(s) >= len(foldedPrefix) && equalFolded(foldedPrefix, s[:len(foldedPrefix)])
}

func hasSuffixFolded(s, foldedSuffix string) bool {
	return len(s) >= len(foldedSuffix) && equalFolded(foldedSuffix, s[len(s)-len(foldedSuffix):])
}

func indexFolded(s, foldedNeedle string) int {
	if foldedNeedle == "" {
		return 0
	}
	for i := 0; i+len(foldedNeedle) <= len(s); i++ {
		if foldByte(s[i]) != foldedNeedle[0] {
			continue
		}
		if equalFolded(foldedNeedle, s[i:i+len(foldedNeedle)]) {
			return i
		}
	}
	return -1
}

// Keep returns surviving tags in order without mutating tags. It returns tags
// itself when nothing is removed.
func (f *Filters) Keep(tags []string) []string {
	if f == nil {
		return tags
	}
	for i, tag := range tags {
		if f.Retains(tag) {
			continue
		}
		kept := make([]string, i, len(tags)-1)
		copy(kept, tags[:i])
		for _, t := range tags[i+1:] {
			if f.Retains(t) {
				kept = append(kept, t)
			}
		}
		return kept
	}
	return tags
}

// Retains reports whether a single "key:value" tag survives f.
func (f *Filters) Retains(tag string) bool {
	if f == nil {
		return true
	}
	key, value, ok := splitTag(tag)
	if !ok {
		return true
	}
	return f.RetainsTag(key, value)
}

// RetainsTag reports whether a tag with the provided key and value survives f.
func (f *Filters) RetainsTag(key, value string) bool {
	return f == nil || f.decide(key, value) != dropDecision
}

// IsEmpty reports whether f would remove nothing.
func (f *Filters) IsEmpty() bool {
	return f == nil || len(f.patterns.Exclude) == 0
}

// Patterns returns the compiled pattern set for the status page.
func (f *Filters) Patterns() Patterns {
	if f == nil {
		return Patterns{}
	}
	return f.patterns
}

// Scoped applies source-level rules ahead of global rules for one log source.
type Scoped struct {
	source *Filters
	global *Filters
}

// NewScoped pairs source and global filters, returning nil when both are inert.
func NewScoped(global, source *Filters) *Scoped {
	if global.IsEmpty() && source.IsEmpty() {
		return nil
	}
	if global.IsEmpty() {
		return &Scoped{source: source}
	}
	if source.Patterns().IsEmpty() {
		return &Scoped{global: global}
	}
	return &Scoped{source: source, global: global}
}

// Keep returns the tags that survive s, preserving order.
func (s *Scoped) Keep(tags []string) []string {
	if s == nil {
		return tags
	}
	for i, tag := range tags {
		if s.Retains(tag) {
			continue
		}
		kept := make([]string, i, len(tags)-1)
		copy(kept, tags[:i])
		for _, t := range tags[i+1:] {
			if s.Retains(t) {
				kept = append(kept, t)
			}
		}
		return kept
	}
	return tags
}

// Retains reports whether a tag survives. Precedence is source include/exclude,
// global include/exclude, then retained by default.
func (s *Scoped) Retains(tag string) bool {
	if s == nil {
		return true
	}
	key, value, ok := splitTag(tag)
	if !ok {
		return true
	}
	return s.RetainsTag(key, value)
}

// RetainsTag reports whether a tag with the provided key and value survives s.
func (s *Scoped) RetainsTag(key, value string) bool {
	if s == nil {
		return true
	}
	if s.source != nil {
		switch s.source.decide(key, value) {
		case keepDecision:
			return true
		case dropDecision:
			return false
		}
	}
	return s.global == nil || s.global.decide(key, value) != dropDecision
}
