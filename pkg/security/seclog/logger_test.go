// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestPatternLoggerLevelChecks(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), "debug")
	assert.True(t, DefaultLogger.IsDebugging())
	assert.False(t, DefaultLogger.IsTracing())

	// debug is enabled at trace level, the checks must not require an exact match
	pkglog.SetupLogger(pkglog.Default(), "trace")
	assert.True(t, DefaultLogger.IsDebugging())
	assert.True(t, DefaultLogger.IsTracing())
}
