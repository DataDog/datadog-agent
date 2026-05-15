// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmtags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestIsTagged(t *testing.T) {
	tests := []struct {
		name       string
		checkName  string
		setupCfg   func(cfg pkgconfigmodel.Config)
		wantResult bool
	}{
		{
			name:      "ccm_mode lightweight, check in tagged list returns true",
			checkName: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "lightweight", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "ccm_mode lightweight, check not in tagged list returns false",
			checkName: "disk",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "lightweight", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
		{
			name:      "ccm_mode lightweight, empty tagged list tags all checks",
			checkName: "any_check",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "lightweight", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{}, pkgconfigmodel.SourceFile)
			},
			wantResult: true,
		},
		{
			name:      "ccm_mode unset, check would be in tagged list",
			checkName: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			tt.setupCfg(cfg)

			result := IsTagged(tt.checkName, cfg)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestApplySenderTags(t *testing.T) {
	tests := []struct {
		name          string
		integration   string
		id            checkid.ID
		setupCfg      func(cfg pkgconfigmodel.Config)
		wantInfraTags []string // nil means AppendInfraTags should not be called
	}{
		{
			name:        "eligible integration appends ccm_mode to sender infra tags",
			integration: "cpu",
			id:          checkid.ID("cpu:abc"),
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "lightweight", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantInfraTags: []string{"ccm_mode:lightweight"},
		},
		{
			name:        "integration not in tagged list leaves infra tags unchanged",
			integration: "disk",
			id:          checkid.ID("disk:def"),
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "lightweight", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantInfraTags: nil,
		},
		{
			name:        "ccm_mode unset does not append infra tags",
			integration: "cpu",
			id:          checkid.ID("cpu:ghi"),
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("ccm_mode", "", pkgconfigmodel.SourceFile)
				cfg.Set("integration.ccm_lightweight.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantInfraTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockS := mocksender.NewMockSender(tt.id)
			if tt.wantInfraTags != nil {
				mockS.On("AppendInfraTags", tt.wantInfraTags).Return().Once()
			}

			cfg := configmock.New(t)
			tt.setupCfg(cfg)

			ApplySenderTags(mockS.GetSenderManager(), tt.id, tt.integration, cfg)

			mockS.AssertExpectations(t)
		})
	}
}
