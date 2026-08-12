// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestDisableMapPreallocation(t *testing.T) {
	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetInTest("service_monitoring_config.disable_map_preallocation", false)
		cfg := New()

		assert.False(t, cfg.DisableMapPreallocation)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_DISABLE_MAP_PREALLOCATION", "false")
		cfg := New()

		assert.False(t, cfg.DisableMapPreallocation)
	})

	t.Run("default value", func(t *testing.T) {
		kversion, err := kernel.HostVersion()
		require.NoError(t, err)
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, cfg.DisableMapPreallocation, kversion >= kernel.VersionCode(6, 1, 0))
	})
}
