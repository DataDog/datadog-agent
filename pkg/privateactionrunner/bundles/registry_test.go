// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

package privatebundles

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type testBundle struct {
	name string
}

func (b *testBundle) GetAction(string) types.Action {
	return nil
}

func TestRegistryGetBundle(t *testing.T) {
	rootBundle := &testBundle{name: "root"}
	gitlabBranchesBundle := &testBundle{name: "gitlab-branches"}
	nonRootRoutedBundle := &testBundle{name: "non-root-routed"}
	registry := &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.authoredscripts": rootBundle,
			"com.datadoghq.gitlab.branches": gitlabBranchesBundle,
			"com.datadoghq.http":            nonRootRoutedBundle,
		},
	}

	tests := []struct {
		name     string
		bundleID string
		expected types.Bundle
	}{
		{
			name:     "returns exact root bundle",
			bundleID: "com.datadoghq.authoredscripts",
			expected: rootBundle,
		},
		{
			name:     "returns exact nested bundle",
			bundleID: "com.datadoghq.gitlab.branches",
			expected: gitlabBranchesBundle,
		},
		{
			name:     "returns root-routed bundle for nested bundle",
			bundleID: "com.datadoghq.authoredscripts.helm",
			expected: rootBundle,
		},
		{
			name:     "does not route nested bundle for non-root-routed bundle",
			bundleID: "com.datadoghq.http.extra",
			expected: nil,
		},
		{
			name:     "returns nil for unknown bundle",
			bundleID: "com.datadoghq.unknown.extra",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := registry.GetBundle(tt.bundleID)
			if tt.expected == nil {
				assert.Nil(t, actual)
				return
			}
			assert.Same(t, tt.expected, actual)
		})
	}
}
