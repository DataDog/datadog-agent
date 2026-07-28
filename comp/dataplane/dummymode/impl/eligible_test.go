// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package dummymodeimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name string
		// env is applied before the config is built, so it lands in the env var layer.
		env map[string]string
		// setup mutates the config before the assertion; nil means "leave defaults".
		setup func(cfg pkgconfigmodel.Config)
		want  bool
	}{
		{
			name: "defaults are eligible",
			want: true,
		},
		{
			name: "dummy_mode explicitly disabled",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneDummyMode, false, pkgconfigmodel.SourceFile)
			},
			want: false,
		},
		{
			name: "data_plane.enabled explicitly true means ADP already runs for real",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, true, pkgconfigmodel.SourceFile)
			},
			want: false,
		},
		{
			name: "data_plane.enabled explicitly false means the operator opted out",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceFile)
			},
			want: false,
		},
		{
			name: "data_plane.enabled set by env is still explicit",
			env:  map[string]string{"DD_DATA_PLANE_ENABLED": "true"},
			want: false,
		},
		{
			name: "dummy_mode disabled by env",
			env:  map[string]string{"DD_DATA_PLANE_DUMMY_MODE": "false"},
			want: false,
		},
		{
			name: "data_plane.enabled set by fleet policies is still explicit",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceFleetPolicies)
			},
			want: false,
		},
		{
			// This is what the platform gate in pkg/config/setup installs on platforms where
			// ADP cannot run, and on Windows without process_manager.enabled. Expressed as the
			// resulting override rather than by calling that gate, which is unexported there;
			// that it produces a SourceAgentRuntime override is asserted by
			// TestSanitizeDataPlaneConfig in pkg/config/setup.
			name: "a SourceAgentRuntime lock makes it ineligible",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceAgentRuntime)
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := configmock.New(t)
			if tt.setup != nil {
				tt.setup(cfg)
			}
			assert.Equal(t, tt.want, isEligible(cfg))
		})
	}
}
