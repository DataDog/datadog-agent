// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagfilter matches individual log tags against include/exclude globs
// (`logs_config.tag_filters`).
//
// A pattern is `key:value` or a bare `key` (any value). `*` is the only
// metacharacter. Include rescues tags from exclude; include alone is a no-op.
// Patterns that match every tag (`*`, `**`, `*:*`) are rejected. ProtectedKeys
// are never dropped. A compiled *Filters is immutable.
package tagfilter

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var global atomic.Pointer[Filters]

// SetGlobal installs the agent-wide filter. Sources that never pass through
// LogSources.AddSource resolve their filter through it, so it must be installed
// before any log is encoded.
func SetGlobal(f *Filters) {
	global.Store(f)
}

// Global returns the agent-wide filter, or nil when none is configured.
func Global() *Filters {
	return global.Load()
}

// ProtectedKeys are always sent. `status` and `timestamp` are omitted because
// they are encoder fields, not tags.
var ProtectedKeys = []string{
	"source", "service", "host", "hostname",
	"env", "version",
}

var protectedKeySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(ProtectedKeys))
	for _, key := range ProtectedKeys {
		set[strings.ToLower(key)] = struct{}{}
	}
	return set
}()

func isProtectedKey(key string) bool {
	_, ok := protectedKeySet[strings.ToLower(key)]
	return ok
}

type listKind string

const (
	includeList listKind = "include"
	excludeList listKind = "exclude"
)

// Filters is the compiled filter set for one source.
type Filters struct {
	include  []*pattern
	exclude  []*pattern
	warnings []string
}

type pattern struct {
	raw      string
	keySegs  []string
	valSegs  []string
	hasValue bool
}

// Compile builds a Filters from include/exclude patterns. Empty lists compile
// to an identity filter. An exclude naming a protected key warns rather than errors.
func Compile(include, exclude []string) (*Filters, error) {
	inc, incWarnings, err := compileList(include, includeList)
	if err != nil {
		return nil, err
	}
	exc, excWarnings, err := compileList(exclude, excludeList)
	if err != nil {
		return nil, err
	}
	return &Filters{
		include:  inc,
		exclude:  exc,
		warnings: append(incWarnings, excWarnings...),
	}, nil
}

func compileList(raws []string, list listKind) ([]*pattern, []string, error) {
	if len(raws) == 0 {
		return nil, nil, nil
	}
	out := make([]*pattern, 0, len(raws))
	var warnings []string
	seen := make(map[string]struct{}, len(raws))
	for i, raw := range raws {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, nil, fmt.Errorf("tagfilter: %s[%d]: empty tag filter pattern", list, i)
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		p, warning, err := compilePattern(trimmed, list)
		if err != nil {
			return nil, nil, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		out = append(out, p)
	}
	return out, warnings, nil
}

func compilePattern(raw string, list listKind) (*pattern, string, error) {
	// First colon only: tag values can contain colons (e.g. task_arn).
	key, value, hasValue := strings.Cut(raw, ":")
	if key == "" {
		return nil, "", fmt.Errorf("tagfilter: invalid %s pattern %q: missing tag key before %q", list, raw, ":")
	}
	p := &pattern{raw: raw, keySegs: strings.Split(key, "*"), hasValue: hasValue}
	if hasValue {
		p.valSegs = strings.Split(value, "*")
	}
	if p.matchesEveryTag() {
		return nil, "", fmt.Errorf(
			"tagfilter: invalid %s pattern %q: a pattern that matches every tag is not allowed; "+
				"name the tag keys to filter instead", list, raw)
	}
	var warning string
	if list == excludeList {
		warning = protectedKeyWarning(raw, key)
	}
	return p, warning, nil
}

func protectedKeyWarning(raw, key string) string {
	lowerKeySegs := strings.Split(strings.ToLower(key), "*")
	if isAllWildcard(lowerKeySegs) {
		return ""
	}
	for _, protected := range ProtectedKeys {
		if globMatch(lowerKeySegs, strings.ToLower(protected)) {
			return fmt.Sprintf(
				"%s pattern %q matches protected tag key %q, which is always sent and will not be dropped",
				excludeList, raw, protected)
		}
	}
	return ""
}

func isAllWildcard(segs []string) bool {
	if len(segs) < 2 {
		return false
	}
	for _, seg := range segs {
		if seg != "" {
			return false
		}
	}
	return true
}

func (p *pattern) matchesEveryTag() bool {
	if !isAllWildcard(p.keySegs) {
		return false
	}
	return !p.hasValue || isAllWildcard(p.valSegs)
}

func (p *pattern) matches(key, value string, hasValue bool) bool {
	if !globMatch(p.keySegs, key) {
		return false
	}
	if !p.hasValue {
		return true
	}
	if !hasValue {
		return false
	}
	return globMatch(p.valSegs, value)
}

func globMatch(segs []string, s string) bool {
	if len(segs) == 1 {
		return segs[0] == s
	}
	if !strings.HasPrefix(s, segs[0]) {
		return false
	}
	s = s[len(segs[0]):]
	// Consume the head first so overlapping head/tail (`a*a` vs `a`) cannot share characters.
	tail := segs[len(segs)-1]
	if !strings.HasSuffix(s, tail) {
		return false
	}
	s = s[:len(s)-len(tail)]
	for _, seg := range segs[1 : len(segs)-1] {
		if seg == "" {
			continue
		}
		i := strings.Index(s, seg)
		if i < 0 {
			return false
		}
		s = s[i+len(seg):]
	}
	return true
}

// Retains reports whether one tag survives this filter set.
func (f *Filters) Retains(tag string) bool {
	if f.IsEmpty() {
		return true
	}
	key, value, hasValue := strings.Cut(tag, ":")
	return f.retainsSplit(key, value, hasValue)
}

// Apply returns the surviving tags. The returned slice must not be modified.
func (f *Filters) Apply(tags []string) []string {
	if f.IsEmpty() {
		return tags
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		key, value, hasValue := strings.Cut(tag, ":")
		if f.retainsSplit(key, value, hasValue) {
			out = append(out, tag)
		}
	}
	return out
}

func (f *Filters) retainsSplit(key, value string, hasValue bool) bool {
	if isProtectedKey(key) {
		return true
	}
	if matchesAny(f.include, key, value, hasValue) {
		return true
	}
	return !matchesAny(f.exclude, key, value, hasValue)
}

func matchesAny(list []*pattern, key, value string, hasValue bool) bool {
	for _, p := range list {
		if p.matches(key, value, hasValue) {
			return true
		}
	}
	return false
}

// Scoped merges source and global filters by specificity, not by applying them
// in sequence. Per tag: protected key, then source include/exclude, then global
// include/exclude; unmatched tags are kept.
type Scoped struct {
	source *Filters
	global *Filters
}

// NewScoped pairs a source filter with the agent-wide one. Either may be nil.
func NewScoped(global, source *Filters) *Scoped {
	return &Scoped{source: source, global: global}
}

// IsEmpty reports whether neither scope would change anything.
func (s *Scoped) IsEmpty() bool {
	return s == nil || (s.source.IsEmpty() && s.global.IsEmpty())
}

// IsIncludeOnly reports whether the two scopes together configure include
// patterns but no exclude pattern, which drops nothing.
func (s *Scoped) IsIncludeOnly() bool {
	if s == nil {
		return false
	}
	includes := 0
	for _, f := range [2]*Filters{s.source, s.global} {
		if f == nil {
			continue
		}
		if len(f.exclude) > 0 {
			return false
		}
		includes += len(f.include)
	}
	return includes > 0
}

// Retains reports whether one tag survives both scopes.
func (s *Scoped) Retains(tag string) bool {
	if s.IsEmpty() {
		return true
	}
	key, value, hasValue := strings.Cut(tag, ":")
	return s.retainsSplit(key, value, hasValue)
}

// Apply returns the surviving tags. The returned slice must not be modified.
func (s *Scoped) Apply(tags []string) []string {
	if s.IsEmpty() {
		return tags
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		key, value, hasValue := strings.Cut(tag, ":")
		if s.retainsSplit(key, value, hasValue) {
			out = append(out, tag)
		}
	}
	return out
}

func (s *Scoped) retainsSplit(key, value string, hasValue bool) bool {
	return isProtectedKey(key) || s.retains(key, value, hasValue)
}

func (s *Scoped) retains(key, value string, hasValue bool) bool {
	for _, f := range [2]*Filters{s.source, s.global} {
		if f == nil {
			continue
		}
		if matchesAny(f.include, key, value, hasValue) {
			return true
		}
		if matchesAny(f.exclude, key, value, hasValue) {
			return false
		}
	}
	return true
}

// IsEmpty reports whether this filter set would change anything.
func (f *Filters) IsEmpty() bool {
	return f == nil || (len(f.include) == 0 && len(f.exclude) == 0)
}

// IsIncludeOnly reports whether include patterns are configured with no exclude
// patterns for them to rescue tags from, which drops nothing.
func (f *Filters) IsIncludeOnly() bool {
	return f != nil && len(f.include) > 0 && len(f.exclude) == 0
}

// Warnings returns compile-time advisories. A config that produces warnings is still valid.
func (f *Filters) Warnings() []string {
	if f == nil {
		return nil
	}
	return f.warnings
}

// Patterns is the configured filter set, for the status page.
type Patterns struct {
	Include []string
	Exclude []string
}

// IsEmpty reports whether no pattern is configured.
func (p Patterns) IsEmpty() bool { return len(p.Include) == 0 && len(p.Exclude) == 0 }

// Patterns returns the configured filter set. A nil *Filters yields the zero value.
func (f *Filters) Patterns() Patterns {
	if f == nil {
		return Patterns{}
	}
	return Patterns{
		Include: rawPatterns(f.include),
		Exclude: rawPatterns(f.exclude),
	}
}

func rawPatterns(list []*pattern) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.raw)
	}
	return out
}
