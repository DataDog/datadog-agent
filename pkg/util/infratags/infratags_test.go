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
			assert.Equal(t, tt.wantNil, NewTagger(cfg) == nil)
		})
	}
}
