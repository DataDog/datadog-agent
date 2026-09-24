// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

package privatebundles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type testRCClient struct {
	handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

func (c *testRCClient) Subscribe(_ string, handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	c.handler = handler
}

func (*testRCClient) GetConfigTUFProof(string) (state.ConfigTUFProof, bool) {
	return state.ConfigTUFProof{}, false
}

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

func TestNewRegistryWiresAuthoredScriptCatalog(t *testing.T) {
	rcClient := &testRCClient{}
	registry, err := NewRegistry(&config.Config{}, rcClient, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, rcClient.handler)

	bundle := registry.GetBundle("com.datadoghq.authoredscripts.echo")
	require.NotNil(t, bundle)
	action := bundle.GetAction("echo")
	require.NotNil(t, action)
}
