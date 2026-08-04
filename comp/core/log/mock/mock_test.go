// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package mock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// TestNewRestoresPreviousLogLevel asserts New's cleanup puts back the level that was in effect,
// rather than assuming the default one.
func TestNewRestoresPreviousLogLevel(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), pkglog.WarnStr)
	before, err := pkglog.GetLogLevel()
	require.NoError(t, err)
	require.Equal(t, pkglog.WarnStr, before.String())

	t.Run("inner", func(t *testing.T) { New(t) })

	after, err := pkglog.GetLogLevel()
	require.NoError(t, err)
	assert.Equal(t, before.String(), after.String())
}
