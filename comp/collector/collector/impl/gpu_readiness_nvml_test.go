// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package collectorimpl

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func TestGPUReadinessNVML(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lc := compdef.NewTestLifecycle(t)
		c := &collectorImpl{
			config: config.NewMockWithOverrides(t, map[string]interface{}{"gpu.enabled": true}),
			log:    logmock.New(t),
		}
		c.registerGPUReadiness(lc)
		assert.Contains(t, health.GetReady().Unhealthy, "gpu-nvml")

		safenvml.WithMockNVML(t, testutil.NewMockNVML())
		require.NoError(t, lc.Start(context.Background()))
		t.Cleanup(func() { require.NoError(t, lc.Stop(context.Background())) })
		synctest.Wait()
		assert.NotContains(t, health.GetReady().Unhealthy, "gpu-nvml")
	})
}
