// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestParseLogLevelsSpecRoundTrip(t *testing.T) {
	spec := "info,github.com/DataDog/datadog-agent/comp/forwarder/...=debug"
	cfg, err := ParseLogLevels(spec)
	require.NoError(t, err)
	assert.Equal(t, spec, cfg.Spec())
}

func TestGetLogLevelSpecRoundTrip(t *testing.T) {
	spec := "info,github.com/DataDog/datadog-agent/comp/forwarder/...=debug"
	cfg, err := ParseLogLevels(spec)
	require.NoError(t, err)

	SetupLogger(Default(), spec)
	t.Cleanup(func() { SetupLogger(Default(), "info") })

	got, err := GetLogLevelSpec()
	require.NoError(t, err)
	assert.Equal(t, spec, got)
	assert.Equal(t, spec, cfg.Spec())
}

func TestGetLogLevelSpecFallsBackToDefaultLevelForNonParsedConfig(t *testing.T) {
	SetupLogger(Default(), "warn")
	require.NoError(t, ChangeLogLevel(DebugLvl))

	got, err := GetLogLevelSpec()
	require.NoError(t, err)
	assert.Equal(t, "debug", got, "ChangeLogLevel builds a config with no spec string; GetLogLevelSpec must fall back to the canonical default level")
}

func TestParseLogLevelsBareLevel(t *testing.T) {
	cfg, err := ParseLogLevels("debug")
	require.NoError(t, err)

	assert.Equal(t, slog.LevelDebug, cfg.DefaultLevel())
	assert.True(t, cfg.EnabledForPackage("anything", slog.LevelDebug))
}

func TestParseLogLevelsWarningCompat(t *testing.T) {
	cfg, err := ParseLogLevels("warning")
	require.NoError(t, err)
	assert.Equal(t, slog.LevelWarn, cfg.DefaultLevel())
}

func TestParseLogLevelsEmptySpec(t *testing.T) {
	_, err := ParseLogLevels("")
	assert.Error(t, err)

	_, err = ParseLogLevels("   ")
	assert.Error(t, err)
}

func TestParseLogLevelsOnlySeparatorsIsInvalid(t *testing.T) {
	for _, spec := range []string{",", " , ", ",,,", " , , "} {
		_, err := ParseLogLevels(spec)
		assert.Errorf(t, err, "spec %q with no actual instructions must be rejected, not silently default to info", spec)
	}
}

func TestParseLogLevelsInvalidLevel(t *testing.T) {
	_, err := ParseLogLevels("bogus")
	assert.Error(t, err)

	_, err = ParseLogLevels("some/pkg=bogus")
	assert.Error(t, err)
}

func TestParseLogLevelsExactPackage(t *testing.T) {
	cfg, err := ParseLogLevels("info,github.com/DataDog/datadog-agent/comp/forwarder=debug")
	require.NoError(t, err)

	assert.Equal(t, slog.LevelInfo, cfg.DefaultLevel())
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug))
	assert.False(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", slog.LevelDebug),
		"a non-recursive pattern must not apply to subpackages")
}

func TestParseLogLevelsRecursivePackage(t *testing.T) {
	cfg, err := ParseLogLevels("github.com/DataDog/datadog-agent/comp/forwarder/...=debug,info")
	require.NoError(t, err)

	assert.Equal(t, slog.LevelInfo, cfg.DefaultLevel())
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug))
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", slog.LevelDebug))
	assert.False(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/other", slog.LevelDebug))
}

func TestParseLogLevelsRelativePath(t *testing.T) {
	cfg, err := ParseLogLevels("./comp/forwarder/...=debug")
	require.NoError(t, err)

	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug))
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", slog.LevelDebug))
}

func TestParseLogLevelsRelativeExact(t *testing.T) {
	cfg, err := ParseLogLevels("./comp/forwarder=debug")
	require.NoError(t, err)

	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug))
	assert.False(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", slog.LevelDebug))
}

func TestParseLogLevelsDotAlone(t *testing.T) {
	cfg, err := ParseLogLevels(".=debug")
	require.NoError(t, err)

	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent", slog.LevelDebug))
	assert.False(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug),
		"'.' alone (non-recursive) should not apply to subpackages")
}

func TestParseLogLevelsDotRecursive(t *testing.T) {
	cfg, err := ParseLogLevels("./...=debug,info")
	require.NoError(t, err)

	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent", slog.LevelDebug))
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", slog.LevelDebug))
}

func TestParseLogLevelsMultipleInstructionsMostSpecificWins(t *testing.T) {
	cfg, err := ParseLogLevels("info,./comp/...=error,./comp/forwarder/...=debug")
	require.NoError(t, err)

	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder", slog.LevelDebug),
		"the more specific comp/forwarder/... rule should win over comp/...")
	assert.False(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/other", slog.LevelDebug),
		"comp/other is only covered by the less permissive comp/... rule")
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/other", slog.LevelError))
	assert.True(t, cfg.EnabledForPackage("some/unrelated/pkg", slog.LevelInfo), "unrelated packages fall back to the default level")
	assert.False(t, cfg.EnabledForPackage("some/unrelated/pkg", slog.LevelDebug))
}

func TestParseLogLevelsMultipleDefaultLevelsError(t *testing.T) {
	_, err := ParseLogLevels("debug,info")
	assert.Error(t, err)
}

func TestParseLogLevelsEmptyPatternError(t *testing.T) {
	_, err := ParseLogLevels("=debug")
	assert.Error(t, err)
}

func TestParseLogLevelsWhitespaceAndEmptyInstructionsIgnored(t *testing.T) {
	cfg, err := ParseLogLevels(" debug , ./comp/forwarder/... = trace , ")
	require.NoError(t, err)

	assert.Equal(t, slog.LevelDebug, cfg.DefaultLevel())
	assert.True(t, cfg.EnabledForPackage("github.com/DataDog/datadog-agent/comp/forwarder", types.ToSlogLevel(TraceLvl)))
}

func TestParseLogLevelsNoRulesMatchesNoRulesConfig(t *testing.T) {
	// A plain level spec must produce a config with no rules at all, so that
	// Levels never resolves the caller's package for the common case.
	cfg, err := ParseLogLevels("debug")
	require.NoError(t, err)

	levels := types.NewLevels(cfg)
	assert.True(t, levels.EnabledForPC(0, slog.LevelDebug))
	assert.False(t, levels.EnabledForPC(0, slog.LevelDebug-1))
}
