// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestNewKeysManagerProvidesRCListener(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION", "false")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		privateactionrunner.PAREnabled: true,
	})

	provides := newKeysManager(cfg)
	callback, found := provides.Listener[state.ProductActionPlatformRunnerKeys]
	require.True(t, found)

	callback(nil, func(string, state.ApplyStatus) {})
	provides.Manager.WaitForReady()
}

func TestNewKeysManagerDoesNotRegisterWhenPARIsDisabled(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION", "false")
	cfg := coreconfig.NewMock(t)

	provides := newKeysManager(cfg)

	assert.Empty(t, provides.Listener)
}
