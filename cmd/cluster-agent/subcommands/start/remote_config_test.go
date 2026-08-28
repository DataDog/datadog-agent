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
		"actions": map[string]interface{}{
			"api_key":   " api-key ",
			"rc_dd_url": "https://config.extra.datadoghq.com",
			"products":  []interface{}{state.ProductK8SActions},
		},
	})

	specs, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "actions", specs[0].Name)
	assert.Equal(t, "api-key", specs[0].APIKey)
	assert.Equal(t, "https://config.extra.datadoghq.com", specs[0].RCDDURL)
	assert.Equal(t, "remote-config-actions.db", specs[0].DatabaseFileName)
	assert.Equal(t, []string{state.ProductK8SActions}, specs[0].Products)
}

func TestAdditionalRemoteConfigClientSpecsRejectDuplicateDatabaseFiles(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"first": map[string]interface{}{
			"api_key":            "api-key",
			"rc_dd_url":          "https://config.extra.datadoghq.com",
			"products":           []string{state.ProductK8SActions},
			"database_file_name": defaultRemoteConfigDatabaseFileName,
		},
	})

	_, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.ErrorContains(t, err, "database_file_name")
}

func TestAdditionalRemoteConfigClientSpecsRejectDatabaseFileNamePaths(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"first": map[string]interface{}{
			"api_key":            "api-key",
			"rc_dd_url":          "https://config.extra.datadoghq.com",
			"products":           []string{state.ProductK8SActions},
			"database_file_name": "x/../remote-config.db",
		},
	})

	_, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.ErrorContains(t, err, "must be a basename")
}

func TestAdditionalRemoteConfigClientSpecsRejectProcessLevelProducts(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(additionalRemoteConfigClientsConfig, map[string]interface{}{
		"process": map[string]interface{}{
			"api_key":   "api-key",
			"rc_dd_url": "https://config.extra.datadoghq.com",
			"products":  []string{state.ProductAgentConfig},
		},
	})

	_, err := getAdditionalRemoteConfigClientSpecs(cfg)
	require.ErrorContains(t, err, "process-level product")
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

func TestRemoteConfigClientRegistryRoutesProducts(t *testing.T) {
	defaultClient := &rcclient.Client{}
	extraClient := &rcclient.Client{}
	registry := &remoteConfigClientRegistry{
		defaultClient: defaultClient,
		byProduct: map[string]*rcclient.Client{
			state.ProductK8SActions: extraClient,
		},
		productOwners: map[string]string{
			state.ProductK8SActions: "actions",
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
	registry := &remoteConfigClientRegistry{
		defaultClient: defaultClient,
		byProduct: map[string]*rcclient.Client{
			state.ProductContainerAutoscalingSettings: autoscalingClient,
			state.ProductContainerAutoscalingValues:   autoscalingClient,
			state.ProductClusterAutoscalingValues:     autoscalingClient,
		},
		productOwners: map[string]string{
			state.ProductContainerAutoscalingSettings: "autoscaling",
			state.ProductContainerAutoscalingValues:   "autoscaling",
			state.ProductClusterAutoscalingValues:     "autoscaling",
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
	registry := &remoteConfigClientRegistry{
		defaultClient: defaultClient,
		byProduct: map[string]*rcclient.Client{
			state.ProductContainerAutoscalingSettings: extraClient,
		},
		productOwners: map[string]string{
			state.ProductContainerAutoscalingSettings: "autoscaling",
		},
	}

	_, err := registry.ClientForProducts(state.ProductContainerAutoscalingSettings, state.ProductContainerAutoscalingValues)
	require.ErrorContains(t, err, "different clients")
}

func TestRemoteConfigClientRegistryRejectsMixedAutoscalingProductOwnership(t *testing.T) {
	defaultClient := &rcclient.Client{}
	extraClient := &rcclient.Client{}
	registry := &remoteConfigClientRegistry{
		defaultClient: defaultClient,
		byProduct: map[string]*rcclient.Client{
			state.ProductContainerAutoscalingSettings: extraClient,
			state.ProductContainerAutoscalingValues:   extraClient,
		},
		productOwners: map[string]string{
			state.ProductContainerAutoscalingSettings: "autoscaling",
			state.ProductContainerAutoscalingValues:   "autoscaling",
		},
	}

	_, err := registry.ClientForProducts(
		state.ProductContainerAutoscalingSettings,
		state.ProductContainerAutoscalingValues,
		state.ProductClusterAutoscalingValues,
	)
	require.ErrorContains(t, err, "different clients")
}
