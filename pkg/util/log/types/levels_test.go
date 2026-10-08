// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevelRulesSpecRoundTrip(t *testing.T) {
	cfg, err := ParseLevelRules("info,some/pkg=debug", "github.com/DataDog/datadog-agent")
	require.NoError(t, err)
	assert.Equal(t, "info,some/pkg=debug", cfg.Spec())

	plain := NewLevelRules(InfoLvl)
	assert.Equal(t, "", plain.Spec(), "a config built directly has no spec string")
}

func TestLevelRulesNoRules(t *testing.T) {
	cfg := NewLevelRules(InfoLvl)

	assert.Equal(t, InfoLvl, cfg.DefaultLevel())
	assert.True(t, shouldLog(cfg, "any/pkg", InfoLvl))
	assert.False(t, shouldLog(cfg, "any/pkg", DebugLvl))
}

func TestLevelRulesExactMatch(t *testing.T) {
	cfg := mustParseLevelRules(t, "info,a/b=debug")

	assert.True(t, shouldLog(cfg, "a/b", DebugLvl), "exact match should get the override")
	assert.False(t, shouldLog(cfg, "a/b/c", DebugLvl), "non-recursive rule should not apply to subpackages")
	assert.True(t, shouldLog(cfg, "a/b/c", InfoLvl), "subpackage falls back to the default level")
	assert.False(t, shouldLog(cfg, "other", DebugLvl), "unrelated package uses the default level")
}

func TestLevelRulesRecursiveMatch(t *testing.T) {
	cfg := mustParseLevelRules(t, "info,a/b/...=debug")

	assert.True(t, shouldLog(cfg, "a/b", DebugLvl), "recursive rule matches the package itself")
	assert.True(t, shouldLog(cfg, "a/b/c", DebugLvl), "recursive rule matches a subpackage")
	assert.True(t, shouldLog(cfg, "a/b/c/d", DebugLvl), "recursive rule matches a deeper subpackage")
	assert.False(t, shouldLog(cfg, "a/bc", DebugLvl), "must not match on a bare string prefix that isn't a path segment")
	assert.False(t, shouldLog(cfg, "other", DebugLvl))
}

func TestLevelRulesMostSpecificWins(t *testing.T) {
	cfg := mustParseLevelRules(t, "info,a/...=error,a/b/...=debug")

	assert.True(t, shouldLog(cfg, "a/b/c", DebugLvl), "the longer prefix (a/b) should win over the shorter one (a)")
	assert.True(t, shouldLog(cfg, "a/x", ErrorLvl), "packages under 'a' but not 'a/b' use the shorter rule")
	assert.False(t, shouldLog(cfg, "a/x", WarnLvl))
}

func TestLevelRulesExactBeatsRecursiveAtSamePrefix(t *testing.T) {
	cfg := mustParseLevelRules(t, "info,a/b/...=debug,a/b=error")

	assert.False(t, shouldLog(cfg, "a/b", DebugLvl), "the exact rule is more specific and should win for the package itself")
	assert.True(t, shouldLog(cfg, "a/b", ErrorLvl))
	assert.True(t, shouldLog(cfg, "a/b/c", DebugLvl), "subpackages are unaffected by the exact rule and still use the recursive one")
}

func TestLevelRulesDuplicateRuleLastWins(t *testing.T) {
	cfg := mustParseLevelRules(t, "info,a/b=debug,a/b=error")

	assert.True(t, shouldLog(cfg, "a/b", ErrorLvl))
	assert.False(t, shouldLog(cfg, "a/b", DebugLvl), "the later duplicate instruction should override the earlier one")
}

func TestParseLevelRulesRelativePath(t *testing.T) {
	cfg := mustParseLevelRules(t, "./comp/forwarder/...=debug")

	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder", DebugLvl))
	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", DebugLvl))
}

func TestParseLevelRulesRelativeExact(t *testing.T) {
	cfg := mustParseLevelRules(t, "./comp/forwarder=debug")

	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder", DebugLvl))
	assert.False(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", DebugLvl))
}

func TestParseLevelRulesDotAlone(t *testing.T) {
	cfg := mustParseLevelRules(t, ".=debug")

	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent", DebugLvl))
	assert.False(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder", DebugLvl),
		"'.' alone (non-recursive) should not apply to subpackages")
}

func TestParseLevelRulesDotRecursive(t *testing.T) {
	cfg := mustParseLevelRules(t, "./...=debug,info")

	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent", DebugLvl))
	assert.True(t, shouldLog(cfg, "github.com/DataDog/datadog-agent/comp/forwarder", DebugLvl))
}

func TestLevelRulesLevelForPCNoRules(t *testing.T) {
	cfg := NewLevelRules(WarnLvl)
	assert.Equal(t, WarnLvl, cfg.LevelForPC(callerPC(t)))
	assert.Equal(t, WarnLvl, cfg.LevelForPC(0))
}

func TestLevelRulesLevelForPCWithRules(t *testing.T) {
	pc := callerPC(t)

	cfg := mustParseLevelRules(t, "error,github.com/DataDog/datadog-agent/pkg/util/log/types/...=debug")

	assert.Equal(t, DebugLvl, cfg.LevelForPC(pc), "this test's own package should be covered by the recursive rule")
	assert.Equal(t, ErrorLvl, cfg.LevelForPC(0), "an unresolvable pc should fall back to the default level")
}

func TestPackageFromFuncName(t *testing.T) {
	testCases := []struct {
		name     string
		funcName string
		want     string
	}{
		{"plain function", "github.com/DataDog/datadog-agent/comp/forwarder.Start", "github.com/DataDog/datadog-agent/comp/forwarder"},
		{"method on pointer receiver", "github.com/DataDog/datadog-agent/comp/forwarder.(*Forwarder).Start", "github.com/DataDog/datadog-agent/comp/forwarder"},
		{"closure", "github.com/DataDog/datadog-agent/pkg/util/log.SetupLogger.func1", "github.com/DataDog/datadog-agent/pkg/util/log"},
		{"top-level package, no slash", "main.main", "main"},
		// The Go toolchain percent-escapes "." within a package's own name
		// (the last import path element) to disambiguate it from the "."
		// separating the package from the function name, e.g. for a
		// versioned import path like ".../lib.v2" or "gopkg.in/yaml.v3".
		{"versioned package name", "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib%2ev2.FooV2", "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2"},
		{"third-party versioned import path", "gopkg.in/yaml%2ev3.Marshal", "gopkg.in/yaml.v3"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, packageFromFuncName(tc.funcName))
		})
	}
}

func shouldLog(cfg *LevelRules, pkg string, level LogLevel) bool {
	return cfg.levelForPackage(pkg) <= level
}

func mustParseLevelRules(t *testing.T, spec string) *LevelRules {
	t.Helper()
	cfg, err := ParseLevelRules(spec, "github.com/DataDog/datadog-agent")
	require.NoError(t, err)
	return cfg
}

// callerPC returns a PC identifying this function's own call site, the way
// runtime.Callers would capture it for a log call made directly from a test.
func callerPC(t *testing.T) uintptr {
	t.Helper()
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	require.Equal(t, 1, n)
	return pcs[0]
}
