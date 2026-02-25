// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
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
			name: "returns special actions for sibling bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.apps": sets.New[string]("action3"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
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
				"com.datadoghq.kubernetes.apps": sets.New[string]("action4"),
				"com.datadoghq.ddagent":         sets.New[string]("action5"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script":          sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":    sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
				"com.datadoghq.ddagent":         sets.New[string]("testConnection"),
			},
		},
		{
			name: "returns empty for similar looking bundle	",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.dd":           sets.New[string]("action1"),
				"com.datadoghq.dd.subbundle": sets.New[string]("action2"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBundleInheritedAllowedActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedInheritedActions, result)
		})
	}
}
