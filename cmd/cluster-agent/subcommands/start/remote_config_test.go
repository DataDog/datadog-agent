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
