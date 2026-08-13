// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupTestLogger(t *testing.T) {
	t.Helper()
	log.SetupLogger(log.Default(), "info")
	t.Cleanup(func() { require.NoError(t, log.ChangeLogLevel(log.InfoLvl)) })
}

func TestIsTracingRespectsPerPackageOverride(t *testing.T) {
	setupTestLogger(t)

	cfg, err := log.ParseLogLevels("info,github.com/DataDog/datadog-agent/pkg/security/seclog/...=trace")
	require.NoError(t, err)
	require.NoError(t, log.ChangeLogLevels(cfg))

	l := &PatternLogger{}
	assert.True(t, l.IsTracing(), "the global default is info, but this package is overridden to trace")
}

func TestIsDebuggingTrueAtTraceLevelToo(t *testing.T) {
	setupTestLogger(t)
	require.NoError(t, log.ChangeLogLevel(log.TraceLvl))

	l := &PatternLogger{}
	assert.True(t, l.IsDebugging(), "debug-level output is enabled a fortiori whenever trace is")
}

func TestIsDebuggingFalseWhenLessVerbose(t *testing.T) {
	setupTestLogger(t)
	require.NoError(t, log.ChangeLogLevel(log.WarnLvl))

	l := &PatternLogger{}
	assert.False(t, l.IsDebugging())
	assert.False(t, l.IsTracing())
}
