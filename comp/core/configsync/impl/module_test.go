// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestNewOptionalConfigSync(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.port", 1234, pkgconfigmodel.SourceFile)
		deps.Config.Set("agent_ipc.config_refresh_interval", 30, pkgconfigmodel.SourceFile)
		optConfigSync := newOptionalConfigSync(deps)
		_, ok := optConfigSync.Get()
		require.True(t, ok)
	})

	t.Run("disabled ipc port zero", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.port", 0, pkgconfigmodel.SourceFile)
		optConfigSync := newOptionalConfigSync(deps)
		_, ok := optConfigSync.Get()
		require.False(t, ok)
	})

	t.Run("disabled config refresh interval zero", func(t *testing.T) {
		deps := makeDeps(t)
		deps.Config.Set("agent_ipc.config_refresh_interval", 0, pkgconfigmodel.SourceFile)
		optConfigSync := newOptionalConfigSync(deps)
		_, ok := optConfigSync.Get()
		require.False(t, ok)
	})
}
