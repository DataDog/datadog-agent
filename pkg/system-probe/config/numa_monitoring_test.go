// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestNUMAMonitoringDisabledByDefault(t *testing.T) {
	config := mock.NewSystemProbe(t)
	loaded, err := load()
	require.NoError(t, err)
	require.False(t, loaded.ModuleIsEnabled(NUMAMonitoringModule))
	require.Equal(t, 16, config.GetInt("numa_monitoring.max_resctrl_groups"))
}

func TestNUMAMonitoringEnabled(t *testing.T) {
	config := mock.NewSystemProbe(t)
	config.Set("numa_monitoring.enabled", true, configmodel.SourceUnknown)
	loaded, err := load()
	require.NoError(t, err)
	require.True(t, loaded.ModuleIsEnabled(NUMAMonitoringModule))
}
