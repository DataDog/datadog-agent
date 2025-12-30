// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// resetInfraModeConfig resets the infrastructure mode configuration cache for testing.
func resetInfraModeConfig() {
	infraModeConfigOnce = sync.Once{}
}

func TestIsCheckAllowedByInfraMode(t *testing.T) {
	t.Run("full mode allows all checks", func(t *testing.T) {
		cfg := newTestConf(t)
		cfg.Set("infrastructure_mode", "full", model.SourceFile)
		SetDatadog(cfg)
		resetInfraModeConfig()

		assert.True(t, IsCheckAllowedByInfraMode("cpu"))
		assert.True(t, IsCheckAllowedByInfraMode("any_check"))
	})

	t.Run("non-full mode uses allowlist", func(t *testing.T) {
		cfg := newTestConf(t)
		cfg.Set("infrastructure_mode", "minimal", model.SourceFile)
		cfg.Set("allowed_checks", []string{"cpu", "disk"}, model.SourceFile)
		cfg.Set("excluded_default_checks", []string{"disk"}, model.SourceFile)
		SetDatadog(cfg)
		resetInfraModeConfig()

		assert.True(t, IsCheckAllowedByInfraMode("cpu"))
		assert.False(t, IsCheckAllowedByInfraMode("disk"))    // excluded
		assert.False(t, IsCheckAllowedByInfraMode("network")) // not in allowlist
		assert.True(t, IsCheckAllowedByInfraMode("custom_x")) // custom_ always allowed
	})
}

func TestIsCheckExcludedByInfraMode(t *testing.T) {
	cfg := newTestConf(t)
	cfg.Set("excluded_default_checks", []string{"disk", "io"}, model.SourceFile)
	SetDatadog(cfg)
	resetInfraModeConfig()

	assert.True(t, IsCheckExcludedByInfraMode("disk"))
	assert.True(t, IsCheckExcludedByInfraMode("io"))
	assert.False(t, IsCheckExcludedByInfraMode("cpu"))
}
