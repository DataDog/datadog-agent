// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"log/slog"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
)

// rule assigns a level to a package, and optionally to its subpackages.
type rule struct {
	// prefix is the Go import path the rule applies to.
	prefix string
	// recursive, when true, makes the rule also apply to any subpackage of
	// prefix (i.e. any package whose import path starts with "prefix/").
	// When false, the rule only applies to the package whose import path is
	// exactly prefix.
	recursive bool
	// level is the level enabled for packages matched by this rule.
	level LogLevel
}

// matches reports whether pkg (a Go import path) is selected by the rule.
func (r rule) matches(pkg string) bool {
	if pkg == r.prefix {
		return true
	}
	return r.recursive && strings.HasPrefix(pkg, r.prefix+"/")
}

// ruleKey identifies a rule's selector, ignoring its level: two rules with
// the same key select the exact same set of packages.
type ruleKey struct {
	prefix    string
	recursive bool
}

// LevelRules is an immutable log level configuration: a default level,
// applied to any package not selected by a more specific rule, plus any
// number of per-package overrides.
type LevelRules struct {
	level    LogLevel
	minLevel LogLevel
	// rules is sorted most-specific-first: longer prefixes first, and among
	// rules with an equally long prefix, a non-recursive (exact) rule before
	// a recursive one, so the first matching rule is always the correct one.
	rules []rule
	// spec is the raw specification string this config was parsed from.
	spec string
}

// NewLevelRules returns a LevelRules applying level by default and no
// per-package overrides.
func NewLevelRules(level LogLevel) *LevelRules {
	return newLevelRules(level, level.String())
}

// newLevelRules returns a LevelRules applying level by default and each
// of rules to the packages it selects. When several rules select the same
// package, the most specific one wins: a longer prefix takes priority over a
// shorter one, and for two rules sharing the same prefix, a non-recursive
// rule takes priority over a recursive one. When rules contains more than one
// rule with the same prefix and recursive, the last one wins.
func newLevelRules(level LogLevel, spec string, rules ...rule) *LevelRules {
	deduped := make([]rule, 0, len(rules))
	indexOf := make(map[ruleKey]int, len(rules))
	for _, r := range rules {
		key := ruleKey{r.prefix, r.recursive}
		if i, ok := indexOf[key]; ok {
			deduped[i] = r
			continue
		}
		indexOf[key] = len(deduped)
		deduped = append(deduped, r)
	}

	minLevel := level
	for _, r := range deduped {
		minLevel = min(minLevel, r.level)
	}

	slices.SortStableFunc(deduped, func(a, b rule) int {
		if len(a.prefix) != len(b.prefix) {
			if len(a.prefix) > len(b.prefix) {
				return -1
			}
			return 1
		}
		if a.recursive != b.recursive {
			if !a.recursive && b.recursive {
				return -1
			}
			return 1
		}
		return 0
	})

	return &LevelRules{level: level, minLevel: minLevel, rules: deduped, spec: spec}
}

// DefaultLevel returns the level applied to packages that don't match any
// rule in the configuration.
func (c *LevelRules) DefaultLevel() LogLevel {
	return c.level
}

// MinLevel returns the most permissive level enabled anywhere by this
// configuration.
func (c *LevelRules) MinLevel() LogLevel {
	return c.minLevel
}

// Spec returns the specification string this configuration corresponds to.
func (c *LevelRules) Spec() string {
	return c.spec
}

// LevelForPC returns the effective log level for the call site identified by
// pc, as captured by runtime.Callers.
func (c *LevelRules) LevelForPC(pc uintptr) LogLevel {
	if len(c.rules) == 0 || pc == 0 {
		return c.level
	}
	return c.levelForPackage(packageFromPC(pc))
}

// levelForPackage returns the level configured for pkg: the level of the
// most specific matching rule, or the default level if none match.
func (c *LevelRules) levelForPackage(pkg string) LogLevel {
	for _, r := range c.rules {
		if r.matches(pkg) {
			return r.level
		}
	}
	return c.level
}

// RulesSync is a dynamically updatable LevelRules holder, safe for
// concurrent use. It implements slog.Leveler.
type RulesSync struct {
	rules atomic.Pointer[LevelRules]
}

// NewRulesSync returns a RulesSync initialized with levelRules.
func NewRulesSync(levelRules *LevelRules) *RulesSync {
	s := &RulesSync{}
	s.Store(levelRules)
	return s
}

// NewRulesSyncFromLevel returns a RulesSync initialized with a single
// level applying to all packages.
func NewRulesSyncFromLevel(level LogLevel) *RulesSync {
	return NewRulesSync(NewLevelRules(level))
}

// Store atomically replaces the current LevelRules.
func (s *RulesSync) Store(levelRules *LevelRules) {
	s.rules.Store(levelRules)
}

// Load returns the current LevelRules.
func (s *RulesSync) Load() *LevelRules {
	return s.rules.Load()
}

// Level implements slog.Leveler.
func (s *RulesSync) Level() slog.Level {
	return ToSlogLevel(s.Load().MinLevel())
}

// EnabledForPC reports whether level is enabled for the call site identified
// by pc, as captured by runtime.Callers.
func (s *RulesSync) EnabledForPC(pc uintptr, level slog.Level) bool {
	return ToSlogLevel(s.Load().LevelForPC(pc)) <= level
}

// packageFromPC resolves the Go import path of the package containing the
// function identified by pc.
func packageFromPC(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	return packageFromFuncName(frame.Function)
}

// packageFromFuncName extracts the package import path from a fully
// qualified function name as reported by runtime.Frame.Function, e.g.
// "github.com/DataDog/datadog-agent/comp/forwarder.(*Type).Method" becomes
// "github.com/DataDog/datadog-agent/comp/forwarder".
func packageFromFuncName(funcName string) string {
	lastSlash := strings.LastIndexByte(funcName, '/')
	dot := strings.IndexByte(funcName[lastSlash+1:], '.')
	if dot < 0 {
		return funcName
	}
	pkg := funcName[:lastSlash+1+dot]
	//  "." in a package's own name (e.g. a versioned import path like "yaml.v2")
	// becomes "yaml%2ev2", so unescape to get back the literal import path
	if unescaped, err := url.PathUnescape(pkg); err == nil {
		return unescaped
	}
	return pkg
}
