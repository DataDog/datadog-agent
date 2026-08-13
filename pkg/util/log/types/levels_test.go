// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"log/slog"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevelsConfigSpecRoundTrip(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo)
	assert.Equal(t, "", cfg.Spec(), "a config built without WithSpec has no spec string")

	withSpec := cfg.WithSpec("info,some/pkg=debug")
	assert.Equal(t, "info,some/pkg=debug", withSpec.Spec())
	assert.Equal(t, "", cfg.Spec(), "WithSpec must not mutate the receiver")
}

func TestLevelsConfigNoRules(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo)

	assert.Equal(t, slog.LevelInfo, cfg.DefaultLevel())
	assert.True(t, cfg.EnabledForPackage("any/pkg", slog.LevelInfo))
	assert.False(t, cfg.EnabledForPackage("any/pkg", slog.LevelDebug))
}

func TestLevelsConfigExactMatch(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo, LevelRule{Prefix: "a/b", Level: slog.LevelDebug})

	assert.True(t, cfg.EnabledForPackage("a/b", slog.LevelDebug), "exact match should get the override")
	assert.False(t, cfg.EnabledForPackage("a/b/c", slog.LevelDebug), "non-recursive rule should not apply to subpackages")
	assert.True(t, cfg.EnabledForPackage("a/b/c", slog.LevelInfo), "subpackage falls back to the default level")
	assert.False(t, cfg.EnabledForPackage("other", slog.LevelDebug), "unrelated package uses the default level")
}

func TestLevelsConfigRecursiveMatch(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo, LevelRule{Prefix: "a/b", Recursive: true, Level: slog.LevelDebug})

	assert.True(t, cfg.EnabledForPackage("a/b", slog.LevelDebug), "recursive rule matches the package itself")
	assert.True(t, cfg.EnabledForPackage("a/b/c", slog.LevelDebug), "recursive rule matches a subpackage")
	assert.True(t, cfg.EnabledForPackage("a/b/c/d", slog.LevelDebug), "recursive rule matches a deeper subpackage")
	assert.False(t, cfg.EnabledForPackage("a/bc", slog.LevelDebug), "must not match on a bare string prefix that isn't a path segment")
	assert.False(t, cfg.EnabledForPackage("other", slog.LevelDebug))
}

func TestLevelsConfigMostSpecificWins(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo,
		LevelRule{Prefix: "a", Recursive: true, Level: slog.LevelError},
		LevelRule{Prefix: "a/b", Recursive: true, Level: slog.LevelDebug},
	)

	assert.True(t, cfg.EnabledForPackage("a/b/c", slog.LevelDebug), "the longer prefix (a/b) should win over the shorter one (a)")
	assert.True(t, cfg.EnabledForPackage("a/x", slog.LevelError), "packages under 'a' but not 'a/b' use the shorter rule")
	assert.False(t, cfg.EnabledForPackage("a/x", slog.LevelWarn))
}

func TestLevelsConfigExactBeatsRecursiveAtSamePrefix(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo,
		LevelRule{Prefix: "a/b", Recursive: true, Level: slog.LevelDebug},
		LevelRule{Prefix: "a/b", Recursive: false, Level: slog.LevelError},
	)

	assert.False(t, cfg.EnabledForPackage("a/b", slog.LevelDebug), "the exact rule is more specific and should win for the package itself")
	assert.True(t, cfg.EnabledForPackage("a/b", slog.LevelError))
	assert.True(t, cfg.EnabledForPackage("a/b/c", slog.LevelDebug), "subpackages are unaffected by the exact rule and still use the recursive one")
}

func TestLevelsConfigDuplicateRuleLastWins(t *testing.T) {
	cfg := NewLevelsConfig(slog.LevelInfo,
		LevelRule{Prefix: "a/b", Level: slog.LevelDebug},
		LevelRule{Prefix: "a/b", Level: slog.LevelError},
	)

	assert.True(t, cfg.EnabledForPackage("a/b", slog.LevelError))
	assert.False(t, cfg.EnabledForPackage("a/b", slog.LevelDebug), "the later duplicate instruction should override the earlier one")
}

func TestLevelsConfigMinLevelBound(t *testing.T) {
	// minLevel (used by Levels.Level(), the coarse pre-check) must be the
	// most permissive level across the default and every rule, regardless
	// of which package ends up matching.
	cfg := NewLevelsConfig(slog.LevelError,
		LevelRule{Prefix: "a", Level: slog.LevelDebug},
		LevelRule{Prefix: "b", Level: slog.LevelWarn},
	)

	levels := NewLevels(cfg)
	assert.Equal(t, slog.LevelDebug, levels.Level())
}

func TestLevelsStoreIsAtomicallyVisible(t *testing.T) {
	levels := NewLevels(NewLevelsConfig(slog.LevelInfo))
	assert.False(t, levels.EnabledForPC(0, slog.LevelDebug))

	levels.Store(NewLevelsConfig(slog.LevelDebug))
	assert.True(t, levels.EnabledForPC(0, slog.LevelDebug))
}

func TestLevelsEnabledForPCNoRulesNeverResolvesPackage(t *testing.T) {
	levels := NewLevels(NewLevelsConfig(slog.LevelInfo))

	// pc==0 would make runtime.CallersFrames return an "invalid" empty
	// frame whose Function is "", which would never match any rule. Since
	// there are no rules at all here, EnabledForPC must fall back to the
	// plain level comparison without even trying to resolve it.
	assert.True(t, levels.EnabledForPC(0, slog.LevelInfo))
	assert.False(t, levels.EnabledForPC(0, slog.LevelDebug))
}

func TestLevelsEnabledForPCWithRules(t *testing.T) {
	pc := callerPC(t)

	cfg := NewLevelsConfig(slog.LevelError, LevelRule{Prefix: "github.com/DataDog/datadog-agent/pkg/util/log/types", Recursive: true, Level: slog.LevelDebug})
	levels := NewLevels(cfg)

	assert.True(t, levels.EnabledForPC(pc, slog.LevelDebug), "this test's own package should be covered by the recursive rule")
	assert.False(t, levels.EnabledForPC(0, slog.LevelDebug), "an unresolvable pc should fall back to the default level")
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

// callerPC returns a PC identifying this function's own call site, the way
// runtime.Callers would capture it for a log call made directly from a test.
func callerPC(t *testing.T) uintptr {
	t.Helper()
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	require.Equal(t, 1, n)
	return pcs[0]
}
