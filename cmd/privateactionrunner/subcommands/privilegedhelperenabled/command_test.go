// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelperenabled

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
)

func TestRun(t *testing.T) {
	cfg := configmock.NewMock(t)
	logger := logmock.New(t)

	err := run(logger, cfg)
	var exitCoder interface{ ExitCode() int }
	require.True(t, errors.As(err, &exitCoder), "disabled by default")
	assert.Equal(t, disabledExitCode, exitCoder.ExitCode())

	cfg.SetInTest(par.RestrictedShellPrivilegedEnabled, true)
	assert.NoError(t, run(logger, cfg))
}
