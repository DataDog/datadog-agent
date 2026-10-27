// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"privileged-helper", "--helper", "/opt/datadog-agent/embedded/bin/rshell"},
		run,
		func() {})
}

func TestRunStartsHelperOnlyWhenEnabled(t *testing.T) {
	started := 0
	original := start
	start = func(log.Component, *helperParams) error { started++; return nil }
	t.Cleanup(func() { start = original })

	cfg := config.NewMock(t)
	logger := logmock.New(t)
	params := &helperParams{helperPath: "/opt/datadog-agent/embedded/bin/rshell"}

	require.NoError(t, run(logger, cfg, params))
	assert.Zero(t, started, "the helper must not start while the setting is disabled")

	cfg.SetInTest(par.RestrictedShellPrivilegedEnabled, true)
	require.NoError(t, run(logger, cfg, params))
	assert.Equal(t, 1, started)
}
