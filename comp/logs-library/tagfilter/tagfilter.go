// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagfilter removes user-configured log tags before intake encoding.
// Matching uses case-insensitive key:value patterns with value-only wildcards.
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

// Patterns is the compiled pattern set, for the status page.
type Patterns struct {
	Include []string
	Exclude []string
}

// IsEmpty reports whether the set holds no patterns.
func (p Patterns) IsEmpty() bool {
	return len(p.Include) == 0 && len(p.Exclude) == 0
}

// maxBucketedKeyLen bounds the bucket array so a pathological configured key
// cannot size an allocation. Rule keys longer than this go in long.
const maxBucketedKeyLen = 32

// Filters is a compiled set of include and exclude patterns for one scope.
type Filters struct {
	// byLen indexes key rules by key length, so most lookups miss on an empty
	// bucket. Length is min(maxKeyLen, maxBucketedKeyLen) + 1.
	byLen [][]keyEntry
	// long holds rule keys longer than maxBucketedKeyLen. Normally nil.
	long []keyEntry
	// maxKeyLen is the true longest rule key, including long ones.
	maxKeyLen           int
	hasEffectiveExclude bool
	patterns            Patterns
}

// keyEntry is one literal rule key. key is ASCII-folded; first is key[0],
// folded, so a candidate is rejected on one byte compare.
type keyEntry struct {
	key   string
	first byte
	rules *keyRules
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

// Compile builds a filter, dropping and reporting invalid patterns instead of
// failing the full configuration.
func Compile(include, exclude []string) (*Filters, Report) {
	// Accumulate in a map, then discard it for length buckets once every
	// pattern is in; compilation is not on any hot path.
	byKey := make(map[string]*keyRules, len(protectedKeys))
	for _, key := range protectedKeys {
		byKey[key] = &keyRules{protected: true}
	}

	var report Report
	f := &Filters{}
	f.patterns = Patterns{
		Include: compileList(byKey, include, false, &report),
		Exclude: compileList(byKey, exclude, true, &report),
	}
	for _, rules := range byKey {
		if !rules.protected && (rules.excludeAny || len(rules.excludeVals) > 0) {
			f.hasEffectiveExclude = true
			break
		}
	}
	f.buildBuckets(byKey)
	return f, report
}

// buildBuckets materializes the length-indexed lookup structure from the
// accumulated rules.
func (f *Filters) buildBuckets(byKey map[string]*keyRules) {
	for key := range byKey {
		if len(key) > f.maxKeyLen {
			f.maxKeyLen = len(key)
		}
	}
	// Cap the array because user-configured keys have no length limit.
	f.byLen = make([][]keyEntry, min(f.maxKeyLen, maxBucketedKeyLen)+1)
	for key, rules := range byKey {
		e := keyEntry{key: key, first: key[0], rules: rules}
		if len(key) <= maxBucketedKeyLen {
			f.byLen[len(key)] = append(f.byLen[len(key)], e)
		} else {
			f.long = append(f.long, e)
		}
	}
}

// compileList validates and compiles one pattern list, returning the patterns
// (trimmed, deduplicated) that survived.
func compileList(byKey map[string]*keyRules, patterns []string, isExclude bool, report *Report) []string {
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

		folded := asciiLower(key)
		if isExclude && isProtectedKey(folded) {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"exclude pattern %q targets protected key %q, which can never be removed, so it has no effect",
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

func hasASCIIUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

// asciiLower lowercases ASCII letters in s and returns s unchanged when possible.
func asciiLower(s string) string {
	if !hasASCIIUpper(s) {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// lookupKey resolves a tag key case-insensitively, rejecting most misses by
// length before reading the key bytes.
func (f *Filters) lookupKey(key string) *keyRules {
	n := len(key)
	if n == 0 || n > f.maxKeyLen {
		return nil
	}
	if n >= len(f.byLen) {
		return lookupLong(f.long, key)
	}
	bucket := f.byLen[n]
	if len(bucket) == 0 {
		return nil
	}
	c := foldByte(key[0])
	for i := range bucket {
		if bucket[i].first != c {
			continue
		}
		if equalFolded(bucket[i].key, key) {
			return bucket[i].rules
		}
	}
	return nil
}

// lookupLong scans the overflow bucket, which unlike a length bucket holds keys
// of mixed length and so must compare lengths before folding.
func lookupLong(bucket []keyEntry, key string) *keyRules {
	c := foldByte(key[0])
	for i := range bucket {
		if bucket[i].first != c || len(bucket[i].key) != len(key) {
			continue
		}
		if equalFolded(bucket[i].key, key) {
			return bucket[i].rules
		}
	}
	return nil
}

func foldByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		c += 'a' - 'A'
	}
	return c
}

// equalFolded reports whether s case-folds to folded.
func equalFolded(folded, s string) bool {
	if len(folded) != len(s) {
		return false
	}
	for i := range s {
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
	kr := f.lookupKey(key)
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
	if f == nil {
		return true
	}
	return !f.hasEffectiveExclude
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
	mode   scopedMode
}

type scopedMode uint8

const (
	scopedBoth scopedMode = iota
	scopedSourceOnly
	scopedGlobalOnly
)

// NewScoped pairs source and global filters, returning nil when both are inert.
func NewScoped(global, source *Filters) *Scoped {
	if global.IsEmpty() && source.IsEmpty() {
		return nil
	}
	// Source includes must remain ahead of global excludes; otherwise specialize
	// single-scope filters to avoid a second lookup per tag.
	if global.IsEmpty() {
		return &Scoped{source: source, global: global, mode: scopedSourceOnly}
	}
	if source.Patterns().IsEmpty() {
		return &Scoped{source: source, global: global, mode: scopedGlobalOnly}
	}
	return &Scoped{source: source, global: global, mode: scopedBoth}
}

// Keep returns the tags that survive s, preserving order.
func (s *Scoped) Keep(tags []string) []string {
	if s == nil {
		return tags
	}
	// In the common single-scope cases, delegate once for the whole slice rather
	// than branching on the scope mode again for every tag in Retains.
	if s.mode == scopedGlobalOnly {
		return s.global.Keep(tags)
	}
	if s.mode == scopedSourceOnly {
		return s.source.Keep(tags)
	}
	for i, tag := range tags {
		if s.retainsBoth(tag) {
			continue
		}
		kept := make([]string, i, len(tags)-1)
		copy(kept, tags[:i])
		for _, t := range tags[i+1:] {
			if s.retainsBoth(t) {
				kept = append(kept, t)
			}
		}
		return kept
	}
	return tags
}

// Retains reports whether a tag survives. Precedence is protected key, source
// include/exclude, global include/exclude, then retained by default.
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
	if s.mode == scopedGlobalOnly {
		return s.global.decide(key, value) != dropDecision
	}
	if s.mode == scopedSourceOnly {
		return s.source.decide(key, value) != dropDecision
	}
	return s.retainsBothParts(key, value)
}

// retainsBoth applies the full source-before-global precedence after NewScoped
// has determined that both scopes can affect the outcome.
func (s *Scoped) retainsBoth(tag string) bool {
	key, value, ok := splitTag(tag)
	if !ok {
		return true
	}
	return s.retainsBothParts(key, value)
}

func (s *Scoped) retainsBothParts(key, value string) bool {
	switch s.source.decide(key, value) {
	case keepDecision:
		return true
	case dropDecision:
		return false
	}
	return s.global.decide(key, value) != dropDecision
}
