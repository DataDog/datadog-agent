// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabled(t *testing.T) {
	logger := Disabled()
	require.NotNil(t, logger)

	// These should all be no-ops and not panic
	logger.Trace("trace")
	logger.Tracef("tracef %s", "test")
	logger.Debug("debug")
	logger.Debugf("debugf %s", "test")
	logger.Info("info")
	logger.Infof("infof %s", "test")

	err := logger.Warn("warn")
	assert.Equal(t, "warn", err.Error())

	err = logger.Warnf("warnf %s", "test")
	assert.Equal(t, "warnf test", err.Error())

	err = logger.Error("error")
	assert.Equal(t, "error", err.Error())

	err = logger.Errorf("errorf %s", "test")
	assert.Equal(t, "errorf test", err.Error())

	err = logger.Critical("critical")
	assert.Equal(t, "critical", err.Error())

	err = logger.Criticalf("criticalf %s", "test")
	assert.Equal(t, "criticalf test", err.Error())

	logger.Flush()
	logger.Close()
}
