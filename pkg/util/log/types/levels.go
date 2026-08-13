// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"log/slog"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
)

// LevelRule assigns a level to a package, and optionally to its subpackages.
type LevelRule struct {
	// Prefix is the Go import path the rule applies to.
	Prefix string
	// Recursive, when true, makes the rule also apply to any subpackage of
	// Prefix (i.e. any package whose import path starts with "Prefix/").
	// When false, the rule only applies to the package whose import path is
	// exactly Prefix.
	Recursive bool
	// Level is the level enabled for packages matched by this rule.
	Level slog.Level
}

// matches reports whether pkg (a Go import path) is selected by the rule.
func (r LevelRule) matches(pkg string) bool {
	if pkg == r.Prefix {
		return true
	}
	return r.Recursive && strings.HasPrefix(pkg, r.Prefix+"/")
}

// ruleKey identifies a rule's selector, ignoring its level: two rules with
// the same key select the exact same set of packages.
type ruleKey struct {
	prefix    string
	recursive bool
}

// LevelsConfig is an immutable log level configuration: a default level,
// applied to any package not selected by a more specific rule, plus any
// number of per-package overrides.
type LevelsConfig struct {
	level    slog.Level
	minLevel slog.Level
	// rules is sorted most-specific-first: longer prefixes first, and among
	// rules with an equally long prefix, a non-recursive (exact) rule before
	// a recursive one, so the first matching rule is always the correct one.
	rules []LevelRule
	// spec is the raw specification string this config was parsed from, if
	// any (see WithSpec). It lets callers that only ever deal in strings
	// (e.g. config files, remote-config, HTTP endpoints) round-trip the
	// exact value that was set, including any per-package overrides that
	// DefaultLevel/String() alone cannot represent.
	spec string
}

// NewLevelsConfig returns a LevelsConfig applying level by default, and each
// of rules to the packages it selects. When several rules select the same
// package, the most specific one wins: a longer Prefix takes priority over a
// shorter one, and for two rules sharing the same Prefix, a non-recursive
// rule takes priority over a recursive one. When rules contains more than one
// rule with the same Prefix and Recursive, the last one wins.
func NewLevelsConfig(level slog.Level, rules ...LevelRule) *LevelsConfig {
	minLevel := level

	deduped := make([]LevelRule, 0, len(rules))
	indexOf := make(map[ruleKey]int, len(rules))
	for _, r := range rules {
		key := ruleKey{r.Prefix, r.Recursive}
		if i, ok := indexOf[key]; ok {
			deduped[i] = r
			continue
		}
		indexOf[key] = len(deduped)
		deduped = append(deduped, r)
	}

	for _, r := range deduped {
		if r.Level < minLevel {
			minLevel = r.Level
		}
	}

	sort.SliceStable(deduped, func(i, j int) bool {
		a, b := deduped[i], deduped[j]
		if len(a.Prefix) != len(b.Prefix) {
			return len(a.Prefix) > len(b.Prefix)
		}
		return !a.Recursive && b.Recursive
	})

	return &LevelsConfig{level: level, minLevel: minLevel, rules: deduped}
}

// DefaultLevel returns the level applied to packages that don't match any
// rule in the configuration.
func (c *LevelsConfig) DefaultLevel() slog.Level {
	return c.level
}

// WithSpec returns a copy of c recording spec as the raw specification
// string it was parsed from, retrievable later via Spec().
func (c *LevelsConfig) WithSpec(spec string) *LevelsConfig {
	clone := *c
	clone.spec = spec
	return &clone
}

// Spec returns the raw specification string this config was parsed from, or
// the empty string if it wasn't built via WithSpec (e.g. built directly from
// a slog.Level and LevelRules).
func (c *LevelsConfig) Spec() string {
	return c.spec
}

// levelFor returns the level configured for pkg: the level of the most
// specific matching rule, or the default level if none match.
func (c *LevelsConfig) levelFor(pkg string) slog.Level {
	for _, r := range c.rules {
		if r.matches(pkg) {
			return r.Level
		}
	}
	return c.level
}

// EnabledForPackage reports whether level is enabled for pkg (a Go import
// path).
func (c *LevelsConfig) EnabledForPackage(pkg string, level slog.Level) bool {
	return c.levelFor(pkg) <= level
}

// Levels is a dynamically updatable log level configuration, safe for
// concurrent use. It implements slog.Leveler.
//
// When its current configuration has no per-package rules, Level and
// EnabledForPC behave, and cost, exactly like a *slog.LevelVar: they never
// resolve the caller's package.
type Levels struct {
	config atomic.Pointer[LevelsConfig]
}

// NewLevels returns a Levels holding cfg.
func NewLevels(cfg *LevelsConfig) *Levels {
	l := &Levels{}
	l.Store(cfg)
	return l
}

// Store atomically replaces the current configuration.
func (l *Levels) Store(cfg *LevelsConfig) {
	l.config.Store(cfg)
}

// Config returns the current configuration.
func (l *Levels) Config() *LevelsConfig {
	return l.config.Load()
}

// Level implements slog.Leveler. It returns a lower bound that is safe to
// use without resolving the caller's package: the most permissive level
// enabled anywhere by the current configuration. Use
// Config().DefaultLevel() for the package-agnostic default level instead.
func (l *Levels) Level() slog.Level {
	return l.config.Load().minLevel
}

// EnabledForPC reports whether level is enabled for the call site identified
// by pc, as captured by runtime.Callers. When the current configuration has
// no per-package rules, pc is never resolved.
func (l *Levels) EnabledForPC(pc uintptr, level slog.Level) bool {
	cfg := l.config.Load()
	if len(cfg.rules) == 0 {
		return cfg.level <= level
	}
	return cfg.EnabledForPackage(packageFromPC(pc), level)
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
// "github.com/DataDog/datadog-agent/comp/forwarder". The Go toolchain
// percent-escapes any "." in a package's own name (the last import path
// element, e.g. a versioned import path like "yaml.v2" becomes "yaml%2ev2"),
// precisely so it can't be confused with the "." separating the package from
// the function name; unescape needed to get back the literal import path a
// caller would use in a pattern.
func packageFromFuncName(funcName string) string {
	lastSlash := strings.LastIndexByte(funcName, '/')
	dot := strings.IndexByte(funcName[lastSlash+1:], '.')
	if dot < 0 {
		return funcName
	}
	pkg := funcName[:lastSlash+1+dot]
	if unescaped, err := url.PathUnescape(pkg); err == nil {
		return unescaped
	}
	return pkg
}
