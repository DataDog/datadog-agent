// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestGetBundleInheritedAllowedActions(t *testing.T) {
	tests := []struct {
		name                     string
		actionsAllowlist         map[string]sets.Set[string]
		expectedInheritedActions map[string]sets.Set[string]
	}{
		{
			name: "returns special actions for existing bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("testConnection", "enrichScript"),
			},
		},
		{
			name: "returns empty when bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns empty when bundle has empty set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name:                     "returns empty for empty allowlist",
			actionsAllowlist:         map[string]sets.Set[string]{},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns special actions for multiple matching bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle":              sets.New[string]("otherAction"),
				"com.datadoghq.script":          sets.New[string]("action1"),
				"com.datadoghq.gitlab.users":    sets.New[string]("action2"),
				"com.datadoghq.kubernetes.core": sets.New[string]("action3"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script":          sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":    sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBundleInheritedAllowedActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedInheritedActions, result)
		})
	}
}

func TestGetDatadogSite(t *testing.T) {
	tests := []struct {
		name         string
		ddURL        string
		site         string
		expectedSite string
	}{
		{
			name:         "dd_url takes precedence and site is extracted",
			ddURL:        "https://app.datadoghq.eu",
			site:         "datadoghq.com",
			expectedSite: "datadoghq.eu",
		},
		{
			name:         "dd_url with US3 site",
			ddURL:        "https://app.us3.datadoghq.com",
			site:         "",
			expectedSite: "us3.datadoghq.com",
		},
		{
			name:         "site config used when dd_url not set",
			ddURL:        "",
			site:         "datadoghq.eu",
			expectedSite: "datadoghq.eu",
		},
		{
			name:         "default site used when neither dd_url nor site is set",
			ddURL:        "",
			site:         "",
			expectedSite: setup.DefaultSite,
		},
		{
			name:         "site config used when dd_url is empty string",
			ddURL:        "",
			site:         "us5.datadoghq.com",
			expectedSite: "us5.datadoghq.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tt.ddURL != "" {
				cfg.SetWithoutSource("dd_url", tt.ddURL)
			}
			if tt.site != "" {
				cfg.SetWithoutSource("site", tt.site)
			}

			result := getDatadogSite(cfg)
			assert.Equal(t, tt.expectedSite, result)
		})
	}
}
