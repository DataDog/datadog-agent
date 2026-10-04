// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/stretchr/testify/assert"
)

func TestResolveInfrastructureMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{"unset falls back to full", "", "full"},
		{"full", "full", "full"},
		{"basic", "basic", "basic"},
		{"end_user_device", "end_user_device", "end_user_device"},
		{"cloud_cost_only", "cloud_cost_only", "cloud_cost_only"},
		{"none", "none", "none"},
		{"typo falls back to full", "cloud_cost_onlyy", "full"},
		{"setting name is not a mode", "infrastructure_mode", "full"},
		{"casing is not normalized", "Cloud_Cost_Only", "full"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("infrastructure_mode", tc.mode)
			assert.Equal(t, tc.expected, ResolveInfrastructureMode(cfg))
		})
	}
}

func TestInfraModeTags(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected []string
	}{
		{"cloud_cost_only is marked", "cloud_cost_only", []string{"infra_mode:cloud_cost_only"}},
		{"end_user_device is marked", "end_user_device", []string{"infra_mode:end_user_device"}},
		{"full is not marked", "full", nil},
		{"unset is not marked", "", nil},
		{"basic is not marked", "basic", nil},
		{"none is not marked", "none", nil},
		{"an invalid mode resolves to full and is not marked", "ccm_only", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("infrastructure_mode", tc.mode)
			assert.Equal(t, tc.expected, InfraModeTags(cfg))
		})
	}
}

// Every marked mode must be a declared mode, otherwise the allowlist marks a
// value that ResolveInfrastructureMode rejects and the mark never ships.
func TestMarkedInfraModesAreKnown(t *testing.T) {
	for _, mode := range markedInfraModes {
		assert.True(t, constants.IsKnownInfraMode(mode), "%q is not a declared infrastructure mode", mode)
	}
}
