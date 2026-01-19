// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestIsCheckAllowed(t *testing.T) {
	tests := []struct {
		name       string
		checkName  string
		setupCfg   func(cfg pkgconfigmodel.Config)
		wantResult bool
	}{
		{
			name:      "integration disabled returns false",
			checkName: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", false, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
		{
			name:      "check in excluded list returns false",
			checkName: "disk",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "full", pkgconfigmodel.SourceFile)
				cfg.Set("integration.excluded", []string{"disk", "memory"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
		{
			name:      "custom check is always allowed",
			checkName: "custom_mycheck",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "basic", pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "check in allowed list for basic mode returns true",
			checkName: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "basic", pkgconfigmodel.SourceFile)
				cfg.Set("integration.basic.allowed", []string{"cpu", "memory"}, pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "check not in allowed list for basic mode returns false",
			checkName: "postgres",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "basic", pkgconfigmodel.SourceFile)
				cfg.Set("integration.basic.allowed", []string{"cpu", "memory"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
		{
			name:      "check in additional list returns true",
			checkName: "postgres",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "basic", pkgconfigmodel.SourceFile)
				cfg.Set("integration.basic.allowed", []string{"cpu", "memory"}, pkgconfigmodel.SourceFile)
				cfg.Set("integration.additional", []string{"postgres"}, pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "excluded check takes precedence over custom prefix",
			checkName: "custom_excluded",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "full", pkgconfigmodel.SourceFile)
				cfg.Set("integration.excluded", []string{"custom_excluded"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
		{
			name:      "end_user_device mode allows all checks",
			checkName: "any_check",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "end_user_device", pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "excluded takes precedence over allowed",
			checkName: "disk",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("integration.enabled", true, pkgconfigmodel.SourceFile)
				cfg.Set("infrastructure_mode", "basic", pkgconfigmodel.SourceFile)
				cfg.Set("integration.basic.allowed", []string{"cpu", "disk", "memory"}, pkgconfigmodel.SourceFile)
				cfg.Set("integration.excluded", []string{"disk"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			tt.setupCfg(cfg)

			result := IsCheckAllowed(tt.checkName, cfg)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}
