// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infratags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestNewTagger(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		taggedChecks []string
		wantNil      bool
	}{
		{"cloud_cost_only with empty allow-list returns non-nil", "cloud_cost_only", []string{}, false},
		{"cloud_cost_only with allow-list returns non-nil", "cloud_cost_only", []string{"cpu"}, false},
		{"full mode returns nil", "full", nil, true},
		{"unknown mode returns nil", "some_future_mode", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("infrastructure_mode", tt.mode, pkgconfigmodel.SourceFile)
			if tt.taggedChecks != nil {
				cfg.Set("integration."+tt.mode+".tagged", tt.taggedChecks, pkgconfigmodel.SourceFile)
			}

			if tt.wantNil {
				assert.Nil(t, NewTagger(cfg))
			} else {
				assert.NotNil(t, NewTagger(cfg))
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
			name:      "multiple infra tags appended",
			tagger:    &Tagger{infraModeTags: []string{"a:1", "b:2"}},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod", "a:1", "b:2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantTags, tt.tagger.AppendTags(tt.inputTags))
		})
	}
}
