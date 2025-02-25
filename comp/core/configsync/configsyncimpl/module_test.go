// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigSync(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.port", 1234, pkgconfigmodel.SourceFile)
		deps.Config.Set("agent_ipc.config_refresh_interval", 30, pkgconfigmodel.SourceFile)
		comp, err := newComponent(deps)
		require.NoError(t, err)
		assert.True(t, comp.(configSync).enabled)
	})

	t.Run("disabled ipc port zero", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.port", 0, pkgconfigmodel.SourceFile)
		comp, err := newComponent(deps)
		require.NoError(t, err)
		assert.False(t, comp.(configSync).enabled)
	})

	t.Run("disabled config refresh interval zero", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.config_refresh_interval", 0, pkgconfigmodel.SourceFile)
		comp, err := newComponent(deps)
		require.NoError(t, err)
		assert.False(t, comp.(configSync).enabled)
	})
}
