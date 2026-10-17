// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func TestCheckAPIKeysResolved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keys    []string
		wantErr bool
	}{
		{name: "plain", keys: []string{"abcdef0123456789abcdef0123456789"}},
		{name: "handle", keys: []string{"ENC[api_key]"}, wantErr: true},
		{name: "handle with padding", keys: []string{" \tENC[api_key] "}, wantErr: true},
		{name: "handle in additional endpoint", keys: []string{"abcdef0123456789abcdef0123456789", "ENC[api_key]"}, wantErr: true},
		{name: "empty", keys: []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eps := make([]apicfg.Endpoint, 0, len(tc.keys))
			for _, k := range tc.keys {
				eps = append(eps, apicfg.Endpoint{APIKey: k, ConfigSettingPath: "api_key"})
			}

			err := CheckAPIKeysResolved(eps)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unresolved secret handle")
		})
	}
}

func TestGetConnectionsAPIEndpoints(t *testing.T) {
	const mainKey = "abcdef0123456789abcdef0123456789"
	const extraKey = "0123456789abcdef0123456789abcdef"

	newConfig := func(t *testing.T, sendToMain bool, additional map[string][]string) pkgconfigmodel.BuildableConfig {
		cfg := configmock.New(t)
		cfg.Set("api_key", mainKey, pkgconfigmodel.SourceAgentRuntime)
		cfg.Set("site", "us5.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
		cfg.Set("process_config.connections_send_to_main_endpoint", sendToMain, pkgconfigmodel.SourceAgentRuntime)
		if additional != nil {
			cfg.Set("process_config.additional_endpoints", additional, pkgconfigmodel.SourceAgentRuntime)
		}
		return cfg
	}

	t.Run("default keeps the main endpoint", func(t *testing.T) {
		eps, err := GetConnectionsAPIEndpoints(newConfig(t, true, map[string][]string{
			"https://process.datadoghq.com": {extraKey},
		}))
		require.NoError(t, err)
		require.Len(t, eps, 2)
		assert.Equal(t, mainKey, eps[0].APIKey)
		assert.Equal(t, "process.us5.datadoghq.com.", eps[0].Endpoint.Host)
	})

	t.Run("skips the main endpoint when disabled", func(t *testing.T) {
		eps, err := GetConnectionsAPIEndpoints(newConfig(t, false, map[string][]string{
			"https://process.datadoghq.com": {extraKey},
		}))
		require.NoError(t, err)
		require.Len(t, eps, 1)
		assert.Equal(t, extraKey, eps[0].APIKey)
		assert.Equal(t, "process.datadoghq.com", eps[0].Endpoint.Host)
	})

	t.Run("unset keeps the main endpoint", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("api_key", mainKey, pkgconfigmodel.SourceAgentRuntime)
		cfg.Set("site", "us5.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)

		eps, err := GetConnectionsAPIEndpoints(cfg)
		require.NoError(t, err)
		require.Len(t, eps, 1)
		assert.Equal(t, mainKey, eps[0].APIKey)
	})

	t.Run("rejects an additional endpoint with no scheme", func(t *testing.T) {
		_, err := GetConnectionsAPIEndpoints(newConfig(t, false, map[string][]string{
			"process.datadoghq.com": {extraKey},
		}))
		assert.ErrorIs(t, err, ErrNoConnectionsEndpoint)
	})

	t.Run("rejects config leaving no destination", func(t *testing.T) {
		_, err := GetConnectionsAPIEndpoints(newConfig(t, false, nil))
		assert.ErrorIs(t, err, ErrNoConnectionsEndpoint)
	})

	t.Run("rejects an additional endpoint with an empty key", func(t *testing.T) {
		_, err := GetConnectionsAPIEndpoints(newConfig(t, false, map[string][]string{
			"https://process.datadoghq.com": {""},
		}))
		assert.ErrorIs(t, err, ErrNoConnectionsEndpoint)
	})

	t.Run("process endpoints keep the main endpoint regardless", func(t *testing.T) {
		eps, err := GetAPIEndpoints(newConfig(t, false, map[string][]string{
			"https://process.datadoghq.com": {extraKey},
		}))
		require.NoError(t, err)
		require.Len(t, eps, 2)
		assert.Equal(t, mainKey, eps[0].APIKey)
	})
}
