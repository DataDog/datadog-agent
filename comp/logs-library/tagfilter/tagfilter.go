// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagfilter removes log tags matching user-configured key:value patterns
// before a log is encoded for the intake.
//
// A pattern is "key:value". The key is a literal, case-sensitive exact match; "*"
// is legal only in the value, where it matches zero or more characters. A tag
// carrying no colon is never matched by any pattern.
//
// Filters are immutable once compiled and safe for concurrent use. Every method is
// nil-receiver safe and treats a nil filter as the identity.
package tagfilter

import (
	"fmt"
	"strings"
)

// Keys that can never be removed, whatever the configuration says.
var protectedKeys = []string{"source", "service", "host", "hostname", "env", "version"}

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

// IsEmpty reports whether Compile had nothing to say.
func (r Report) IsEmpty() bool {
	return len(r.Rejected) == 0 && len(r.Warnings) == 0
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
	protected   bool
	includeAny  bool
	includeVals []valueGlob
	excludeAny  bool
	excludeVals []valueGlob
}

// valueGlob matches a tag value split on "*" at compile time.
type valueGlob struct {
	segments []string
}

// Compile builds a filter from include and exclude pattern lists.
//
// Compile never fails: invalid patterns are dropped individually and reported in
// Report, so a malformed configuration degrades to filtering less rather than
// breaking log delivery.
func Compile(include, exclude []string) (*Filters, Report) {
	f := &Filters{byKey: make(map[string]*keyRules, len(protectedKeys))}
	for _, key := range protectedKeys {
		f.byKey[key] = &keyRules{protected: true}
	}

	var report Report
	f.patterns = Patterns{
		Include: compileList(f, include, false, &report),
		Exclude: compileList(f, exclude, true, &report),
	}
	return f, report
}

// compileList validates and compiles one pattern list, returning the patterns
// (trimmed, deduplicated) that survived.
func compileList(f *Filters, patterns []string, isExclude bool, report *Report) []string {
	seen := make(map[string]struct{}, len(patterns))
	kept := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}

		key, value, reason := parsePattern(p)
		if reason != "" {
			report.Rejected = append(report.Rejected, RejectedPattern{Pattern: p, Reason: reason})
			continue
		}

		lowered := strings.ToLower(key)
		if lowered != key {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"pattern %q has an uppercase key; tag keys are normalized to lowercase, compiled as %q",
				p, lowered+":"+value))
		}
		if isExclude && isProtectedKey(lowered) {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"exclude pattern %q targets protected key %q, which can never be removed, so it has no effect",
				p, lowered))
		}

		kr := f.byKey[lowered]
		if kr == nil {
			kr = &keyRules{}
			f.byKey[lowered] = kr
		}
		if value == "*" {
			if isExclude {
				kr.excludeAny = true
			} else {
				kr.includeAny = true
			}
		} else {
			g := valueGlob{segments: strings.Split(value, "*")}
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
		// Suggesting p+":*" would be invalid advice when p already holds a "*",
		// because that "*" would then sit in the key.
		if strings.ContainsRune(p, '*') {
			return "", "", fmt.Sprintf(
				"pattern %q has no key:value separator, and \"*\" is not allowed in the key; "+
					"name the key exactly and wildcard the value instead", p)
		}
		return "", "", fmt.Sprintf(
			"pattern %q has no key:value separator; use %q to match all values for this key", p, p+":*")
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

func isProtectedKey(key string) bool {
	for _, k := range protectedKeys {
		if k == key {
			return true
		}
	}
	return false
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
	kr := f.byKey[key]
	if kr == nil {
		return noDecision
	}
	if kr.protected {
		return keepDecision
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
		return value == segs[0]
	}
	head, tail := segs[0], segs[len(segs)-1]
	if !strings.HasPrefix(value, head) {
		return false
	}
	// Consume the head before matching the tail so overlapping runs (pattern
	// "a*a" against value "a") can't have one character satisfy both ends.
	value = value[len(head):]
	if !strings.HasSuffix(value, tail) {
		return false
	}
	value = value[:len(value)-len(tail)]
	for _, mid := range segs[1 : len(segs)-1] {
		i := strings.Index(value, mid)
		if i < 0 {
			return false
		}
		value = value[i+len(mid):]
	}
	return true
}

// Keep returns the tags that survive f, preserving order.
//
// The returned slice is tags itself when nothing is removed. Keep never mutates
// tags, because callers may hold a shared cached slice.
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
	return f.decide(key, value) != dropDecision
}

// IsEmpty reports whether f would remove nothing.
func (f *Filters) IsEmpty() bool {
	if f == nil {
		return true
	}
	// Only exclude patterns can ever remove a tag; an include-only filter is inert.
	return len(f.patterns.Exclude) == 0
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

// NewScoped pairs a source-level filter with the process-global filter.
//
// It returns nil when neither scope would remove anything, so callers can store a
// nil TagFilter interface rather than a typed nil.
func NewScoped(global, source *Filters) *Scoped {
	if global.IsEmpty() && source.IsEmpty() {
		return nil
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

// Retains reports whether a single "key:value" tag survives s.
//
// Precedence, highest first: protected key, source include, source exclude, global
// include, global exclude, then retained by default. This is not sequential
// application of the two scopes -- a source include must rescue a tag that a global
// exclude would drop.
func (s *Scoped) Retains(tag string) bool {
	if s == nil {
		return true
	}
	key, value, ok := splitTag(tag)
	if !ok {
		return true
	}
	switch s.source.decide(key, value) {
	case keepDecision:
		return true
	case dropDecision:
		return false
	}
	return s.global.decide(key, value) != dropDecision
}

// IsEmpty reports whether s would remove nothing.
func (s *Scoped) IsEmpty() bool {
	return s == nil || (s.source.IsEmpty() && s.global.IsEmpty())
}

// GlobalPatterns returns the process-global pattern set, for the status page.
func (s *Scoped) GlobalPatterns() Patterns {
	if s == nil {
		return Patterns{}
	}
	return s.global.Patterns()
}

// SourcePatterns returns the source-level pattern set, for the status page.
func (s *Scoped) SourcePatterns() Patterns {
	if s == nil {
		return Patterns{}
	}
	return s.source.Patterns()
}
