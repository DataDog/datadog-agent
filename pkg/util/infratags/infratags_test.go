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

func TestNewMetricTagger(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantNil bool
		wantTag string
	}{
		{
			name:    "cloud_cost_only returns non-nil",
			mode:    "cloud_cost_only",
			wantNil: false,
			wantTag: "infra_mode:cloud_cost_only",
		},
		{
			name:    "end_user_device marks eligible metrics",
			mode:    "end_user_device",
			wantNil: false,
			wantTag: "infra_mode:end_user_device",
		},
		{"full mode returns nil", "full", true, ""},
		{"basic mode returns nil", "basic", true, ""},
		{"none mode returns nil", "none", true, ""},
		{"unknown mode returns nil", "some_future_mode", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("infrastructure_mode", tt.mode, pkgconfigmodel.SourceFile)

			tagger := NewMetricTagger(cfg)
			if tt.wantNil {
				assert.Nil(t, tagger)
				return
			}
			assert.NotNil(t, tagger)
			assert.Equal(t, []string{tt.wantTag}, tagger.infraModeTags)
		})
	}
}

func TestIsCheckEligible(t *testing.T) {
	allChecks := &MetricTagger{infraModeTags: []string{InfraModeCloudCostTag}}

	tests := []struct {
		name      string
		tagger    *MetricTagger
		checkName string
		want      bool
	}{
		{"nil receiver returns false", nil, "cpu", false},
		{"empty check name returns false", allChecks, "", false},
		{"custom_ prefix returns false", allChecks, "custom_check", false},
		{"non-custom check returns true", allChecks, "any_integration", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tagger.IsCheckEligible(tt.checkName))
		})
	}
}

func TestMetricTaggerAppendTags(t *testing.T) {
	eudmTag := fmt.Sprintf("%s:%s", configutils.InfraModeTagKey, "end_user_device")
	tests := []struct {
		name      string
		tagger    *MetricTagger
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
			tagger:    &MetricTagger{},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod"},
		},
		{
			name:      "single infra tag appended",
			tagger:    &MetricTagger{infraModeTags: []string{InfraModeCloudCostTag}},
			inputTags: []string{"env:prod"},
			wantTags:  []string{"env:prod", InfraModeCloudCostTag},
		},
		{
			name:      "dedupes when mark already present",
			tagger:    &MetricTagger{infraModeTags: []string{InfraModeCloudCostTag}},
			inputTags: []string{"env:prod", InfraModeCloudCostTag},
			wantTags:  []string{"env:prod", InfraModeCloudCostTag},
		},
		{
			name:      "end_user_device tag appended",
			tagger:    &MetricTagger{infraModeTags: []string{eudmTag}},
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
