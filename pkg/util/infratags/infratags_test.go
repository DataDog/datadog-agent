// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infratags

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestNewTagger(t *testing.T) {
	tests := []struct {
		name             string
		mode             string
		taggedChecks     []string // only applied for cloud_cost_only
		wantNil          bool
		wantTag          string
		wantAllowlistLen int // 0 means taggedChecks map is nil (all non-custom eligible)
	}{
		{
			name:    "cloud_cost_only with empty allow-list returns non-nil",
			mode:    "cloud_cost_only",
			wantNil: false,
			wantTag: "infra_mode:cloud_cost_only",
		},
		{
			name:             "cloud_cost_only with allow-list returns non-nil",
			mode:             "cloud_cost_only",
			taggedChecks:     []string{"cpu"},
			wantNil:          false,
			wantTag:          "infra_mode:cloud_cost_only",
			wantAllowlistLen: 1,
		},
		{
			name:    "end_user_device marks eligible metrics without allowlist",
			mode:    "end_user_device",
			wantNil: false,
			wantTag: "infra_mode:end_user_device",
		},
		{"full mode returns nil", "full", nil, true, "", 0},
		{"basic mode returns nil", "basic", nil, true, "", 0},
		{"none mode returns nil", "none", nil, true, "", 0},
		{"unknown mode returns nil", "some_future_mode", nil, true, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("infrastructure_mode", tt.mode, pkgconfigmodel.SourceFile)
			if tt.mode == "cloud_cost_only" && tt.taggedChecks != nil {
				cfg.Set("integration.cloud_cost_only.tagged", tt.taggedChecks, pkgconfigmodel.SourceFile)
			}

			tagger := NewTagger(cfg)
			if tt.wantNil {
				assert.Nil(t, tagger)
				return
			}
			assert.NotNil(t, tagger)
			assert.Equal(t, []string{tt.wantTag}, tagger.infraModeTags)
			if tt.wantAllowlistLen == 0 {
				assert.Nil(t, tagger.taggedChecks)
			} else {
				assert.Len(t, tagger.taggedChecks, tt.wantAllowlistLen)
			}
		})
	}
}

func TestIsCheckEligible(t *testing.T) {
	allChecks := &Tagger{infraModeTags: []string{InfraModeCloudCostTag}}
	selective := &Tagger{
		infraModeTags: []string{InfraModeCloudCostTag},
		taggedChecks:  map[string]struct{}{"cpu": {}},
	}

	tests := []struct {
		name      string
		tagger    *Tagger
		checkName string
		want      bool
	}{
		{"nil receiver returns false", nil, "cpu", false},
		{"empty check name returns false", allChecks, "", false},
		{"custom_ prefix returns false", allChecks, "custom_check", false},
		{"nil allow-list tags all non-custom checks", allChecks, "any_integration", true},
		{"check in allow-list returns true", selective, "cpu", true},
		{"check not in allow-list returns false", selective, "disk", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tagger.IsCheckEligible(tt.checkName))
		})
	}
}

func TestTaggerAppendTags(t *testing.T) {
	eudmTag := fmt.Sprintf("%s:%s", configutils.InfraModeTagKey, "end_user_device")
	tests := []struct {
		name      string
		tagger    *Tagger
		inputTags []string
		wantTags  []string
	}{
		{
			name:      "nil tagger is no-op",
			tagger:    nil,
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod"},
		},
		{
			name:      "empty infraModeTags is no-op",
			tagger:    &Tagger{},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod"},
		},
		{
			name:      "single infra tag appended",
			tagger:    &Tagger{infraModeTags: []string{InfraModeCloudCostTag}},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod", InfraModeCloudCostTag},
		},
		{
			name:      "dedupes when mark already present",
			tagger:    &Tagger{infraModeTags: []string{InfraModeCloudCostTag}},
			inputTags: []string{"env:prod", InfraModeCloudCostTag},
			wantTags:  []string{"env:prod", InfraModeCloudCostTag},
		},
		{
			name:      "end_user_device tag appended",
			tagger:    &Tagger{infraModeTags: []string{eudmTag}},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod", eudmTag},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantTags, tt.tagger.AppendTags(tt.inputTags))
		})
	}
}
