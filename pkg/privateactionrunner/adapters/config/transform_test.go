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

func TestGetRelevantSpecialActions(t *testing.T) {
	tests := []struct {
		name                   string
		actionsAllowlist       map[string]sets.Set[string]
		expectedSpecialActions map[string]sets.Set[string]
	}{
		{
			name: "returns special actions for existing bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("action1"),
			},
			expectedSpecialActions: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("testconnection", "enrichscript"),
			},
		},
		{
			name: "returns empty when bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle": sets.New[string]("action1"),
			},
			expectedSpecialActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns empty when bundle has empty set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			expectedSpecialActions: map[string]sets.Set[string]{},
		},
		{
			name:                   "returns empty for empty allowlist",
			actionsAllowlist:       map[string]sets.Set[string]{},
			expectedSpecialActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns special actions for multiple matching bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle":              sets.New[string]("otherAction"),
				"com.datadoghq.script":          sets.New[string]("action1"),
				"com.datadoghq.gitlab.users":    sets.New[string]("action2"),
				"com.datadoghq.kubernetes.core": sets.New[string]("action3"),
			},
			expectedSpecialActions: map[string]sets.Set[string]{
				"com.datadoghq.script":          sets.New[string]("testconnection", "enrichscript"),
				"com.datadoghq.gitlab.users":    sets.New[string]("testconnection"),
				"com.datadoghq.kubernetes.core": sets.New[string]("testconnection"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRelevantSpecialActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedSpecialActions, result)
		})
	}
}
