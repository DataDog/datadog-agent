// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !darwin && !windows && kubeapiserver

package start

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestAdditionalRemoteConfigClientSpecs(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"kubeactions": map[string]interface{}{
			"api_key":   " api-key ",
			"rc_dd_url": "https://config.extra.datadoghq.com",
		},
	})

	specs, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "kubeactions", specs[0].Name)
	assert.Equal(t, "api-key", specs[0].APIKey)
	assert.Equal(t, "https://config.extra.datadoghq.com", specs[0].RCDDURL)
	assert.Equal(t, "remote-config-kubeactions.db", specs[0].DatabaseFileName)
	assert.Equal(t, []string{state.ProductK8SActions}, specs[0].Products)
}

func TestAdditionalRemoteConfigClientSpecsRejectDuplicateDatabaseFiles(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"kubeactions": map[string]interface{}{
			"api_key":            "api-key",
			"rc_dd_url":          "https://config.extra.datadoghq.com",
			"database_file_name": defaultRemoteConfigDatabaseFileName,
		},
	})

	_, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.ErrorContains(t, err, "database_file_name")
}

func TestAdditionalRemoteConfigClientSpecsRejectDatabaseFileNamePaths(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"kubeactions": map[string]interface{}{
			"api_key":            "api-key",
			"rc_dd_url":          "https://config.extra.datadoghq.com",
			"database_file_name": "x/../remote-config.db",
		},
	})

	_, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.ErrorContains(t, err, "must be a basename")
}

func TestAdditionalRemoteConfigClientRoots(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.com")
	cfg.SetInTest("remote_configuration.director_root", "global-director-root")

	defaultRoots := defaultRemoteConfigClientRoots(cfg)
	assert.Equal(t, "datadoghq.com", defaultRoots.site)
	assert.Equal(t, "global-director-root", defaultRoots.directorRoot)

	extraRoots := additionalRemoteConfigClientRoots(cfg, additionalRemoteConfigClientSpec{
		Site:         "datadoghq.eu",
		DirectorRoot: "extra-director-root",
	})
	assert.Equal(t, "datadoghq.eu", extraRoots.site)
	assert.Equal(t, "extra-director-root", extraRoots.directorRoot)
}

func TestAdditionalRemoteConfigClientRootsDefaultToGlobalSite(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.com")

	roots := additionalRemoteConfigClientRoots(cfg, additionalRemoteConfigClientSpec{
		DirectorRoot: "extra-director-root",
	})
	assert.Equal(t, "datadoghq.com", roots.site)
	assert.Equal(t, "extra-director-root", roots.directorRoot)
}

func TestInitializeRemoteConfigClientsIsLazy(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"kubeactions": map[string]interface{}{
			"api_key":   "api-key",
			"rc_dd_url": "https://config.extra.datadoghq.com",
		},
	})

	registry, err := initializeRemoteConfigClients(nil, cfg, nil, "cluster-name", "cluster-id", state.ProductAgentConfig, state.ProductK8SActions)
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Empty(t, registry.clients)
	assert.Empty(t, registry.services)

	require.NotNil(t, registry.defaultInstance)
	assert.Nil(t, registry.defaultInstance.client)
	assert.Nil(t, registry.defaultInstance.service)
	assert.Equal(t, []string{state.ProductAgentConfig}, registry.defaultInstance.products)

	extraInstance := registry.byProduct[state.ProductK8SActions]
	require.NotNil(t, extraInstance)
	assert.Nil(t, extraInstance.client)
	assert.Nil(t, extraInstance.service)
	assert.Equal(t, []string{state.ProductK8SActions}, extraInstance.products)
}

func TestRemoteConfigClientRegistryRoutesProducts(t *testing.T) {
	defaultClient := &rcclient.Client{}
	extraClient := &rcclient.Client{}
	defaultInstance := &remoteConfigClientInstance{name: "default", client: defaultClient}
	extraInstance := &remoteConfigClientInstance{name: "actions", client: extraClient}
	registry := &remoteConfigClientRegistry{
		defaultInstance: defaultInstance,
		byProduct: map[string]*remoteConfigClientInstance{
			state.ProductK8SActions: extraInstance,
		},
	}

	client, err := registry.ClientForProducts(state.ProductK8SActions)
	require.NoError(t, err)
	assert.Same(t, extraClient, client)

	client, err = registry.ClientForProducts(state.ProductClusterAutoscalingValues)
	require.NoError(t, err)
	assert.Same(t, defaultClient, client)
}

func TestRemoteConfigClientRegistryRoutesProductsWithoutEndpointOverrideToDefaultClient(t *testing.T) {
	defaultClient := &rcclient.Client{}
	autoscalingClient := &rcclient.Client{}
	defaultInstance := &remoteConfigClientInstance{name: "default", client: defaultClient}
	autoscalingInstance := &remoteConfigClientInstance{name: "autoscaling", client: autoscalingClient}
	registry := &remoteConfigClientRegistry{
		defaultInstance: defaultInstance,
		byProduct: map[string]*remoteConfigClientInstance{
			state.ProductContainerAutoscalingSettings: autoscalingInstance,
			state.ProductContainerAutoscalingValues:   autoscalingInstance,
			state.ProductClusterAutoscalingValues:     autoscalingInstance,
		},
	}

	client, err := registry.ClientForProducts(state.ProductK8SActions, state.ProductAPMTracing, state.ProductApmPolicies)
	require.NoError(t, err)
	assert.Same(t, defaultClient, client)

	client, err = registry.ClientForProducts()
	require.NoError(t, err)
	assert.Same(t, defaultClient, client)
}

func TestRemoteConfigClientRegistryRejectsMixedProductOwnership(t *testing.T) {
	defaultClient := &rcclient.Client{}
	extraClient := &rcclient.Client{}
	defaultInstance := &remoteConfigClientInstance{name: "default", client: defaultClient}
	extraInstance := &remoteConfigClientInstance{name: "autoscaling", client: extraClient}
	registry := &remoteConfigClientRegistry{
		defaultInstance: defaultInstance,
		byProduct: map[string]*remoteConfigClientInstance{
			state.ProductContainerAutoscalingSettings: extraInstance,
		},
	}

	_, err := registry.ClientForProducts(state.ProductContainerAutoscalingSettings, state.ProductContainerAutoscalingValues)
	require.ErrorContains(t, err, "different clients")
}

func TestRemoteConfigClientRegistryRejectsMixedAutoscalingProductOwnership(t *testing.T) {
	defaultClient := &rcclient.Client{}
	extraClient := &rcclient.Client{}
	defaultInstance := &remoteConfigClientInstance{name: "default", client: defaultClient}
	autoscalingInstance := &remoteConfigClientInstance{name: "autoscaling", client: extraClient}
	registry := &remoteConfigClientRegistry{
		defaultInstance: defaultInstance,
		byProduct: map[string]*remoteConfigClientInstance{
			state.ProductContainerAutoscalingSettings: autoscalingInstance,
			state.ProductContainerAutoscalingValues:   autoscalingInstance,
		},
	}

	_, err := registry.ClientForProducts(
		state.ProductContainerAutoscalingSettings,
		state.ProductContainerAutoscalingValues,
		state.ProductClusterAutoscalingValues,
	)
	require.ErrorContains(t, err, "different clients")
}

// TestAdditionalRemoteConfigClientSpecsPresetProducts checks that a client
// named after a known subsystem does not need an explicit products list.
func TestAdditionalRemoteConfigClientSpecsPresetProducts(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"autoscaling": map[string]interface{}{
			"api_key":   "api-key",
			"rc_dd_url": "https://config.extra.datadoghq.com",
		},
	})

	specs, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	// Cluster autoscaling must be included, otherwise enabling it would split
	// the autoscaling subsystem across two clients.
	assert.ElementsMatch(t, []string{
		state.ProductContainerAutoscalingSettings,
		state.ProductContainerAutoscalingValues,
		state.ProductClusterAutoscalingValues,
	}, specs[0].Products)
}

// TestRemoteConfigClientPresetsMatchConsumers guards the preset table against
// drift: every preset product must be one the Cluster Agent actually resolves
// through ClientForProducts, and no product may appear in two presets.
func TestRemoteConfigClientPresetsMatchConsumers(t *testing.T) {
	// Products the Cluster Agent resolves via ClientForProducts (command.go).
	consumed := map[string]struct{}{
		state.ProductContainerAutoscalingSettings: {},
		state.ProductContainerAutoscalingValues:   {},
		state.ProductClusterAutoscalingValues:     {},
		state.ProductK8SActions:                   {},
		state.ProductActionPlatformRunnerKeys:     {},
		state.ProductAPMTracing:                   {},
		state.ProductApmPolicies:                  {},
	}

	owner := map[string]string{}
	for name, products := range remoteConfigClientPresets {
		require.NotEmpty(t, products, "preset %q must not be empty", name)
		for _, product := range products {
			_, isConsumed := consumed[product]
			assert.True(t, isConsumed,
				"preset %q lists %q, which no subsystem resolves via ClientForProducts; it would be subscribed nowhere", name, product)
			_, isProcessLevel := processLevelRemoteConfigProducts[product]
			assert.False(t, isProcessLevel, "preset %q lists process-level product %q", name, product)
			if previous, duplicated := owner[product]; duplicated {
				t.Errorf("product %q is in both preset %q and %q", product, previous, name)
			}
			owner[product] = name
		}
	}

	// A preset name must never collide with the reserved default status entry.
	_, reserved := remoteConfigClientPresets[defaultRemoteConfigStatusInstance]
	assert.False(t, reserved)
}

// TestAdditionalRemoteConfigClientSpecsRejectUnknownName checks that a client
// name without a preset is rejected. The name is the only thing selecting the
// products a client owns, so an unrecognised one would own nothing and never
// be used. This also covers the default client's status key, which is not a
// preset name and so cannot be taken by an additional client.
func TestAdditionalRemoteConfigClientSpecsRejectUnknownName(t *testing.T) {
	for _, name := range []string{"something-else", defaultRemoteConfigStatusInstance} {
		t.Run(name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
				name: map[string]interface{}{
					"api_key":   "api-key",
					"rc_dd_url": "https://config.extra.datadoghq.com",
				},
			})

			_, err := getAdditionalRemoteConfigClientSpecs(cfg)
			require.ErrorContains(t, err, "is not a known client name")
		})
	}
}
