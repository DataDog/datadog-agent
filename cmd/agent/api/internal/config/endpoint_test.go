// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type testCase struct {
	name           string
	authorized     bool
	existing       bool
	expectedStatus int
}

func testConfigValue(t *testing.T, server *httptest.Server, configName string, expectedStatus int) {
	t.Helper()

	resp, err := server.Client().Get(server.URL + "/" + configName)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, expectedStatus, resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return
	}

	var configValue interface{}
	err = json.Unmarshal(body, &configValue)
	require.NoError(t, err)

	require.EqualValues(t, config.Datadog.Get(configName), configValue)
}

func TestConfigEndpoint(t *testing.T) {
	t.Run("core_config", func(t *testing.T) {
		cfg, server := getConfigServer(t, authorizedConfigPathsCore)
		for configName := range authorizedConfigPathsCore {
			var expectedStatus int
			if cfg.IsSet(configName) {
				expectedStatus = http.StatusOK
			} else {
				expectedStatus = http.StatusNotFound
			}
			testConfigValue(t, server, configName, expectedStatus)
		}
	})

	for _, testCase := range []testCase{
		{"authorized_existing_config", true, true, http.StatusOK},
		{"authorized_missing_config", true, false, http.StatusNotFound},
		{"unauthorized_existing_config", false, true, http.StatusForbidden},
		{"unauthorized_missing_config", false, false, http.StatusForbidden},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configName := "my.config.value"
			authorizedConfigPaths := authorizedSet{}
			if testCase.authorized {
				authorizedConfigPaths[configName] = struct{}{}
			}
			cfg, server := getConfigServer(t, authorizedConfigPaths)
			if testCase.existing {
				cfg.SetWithoutSource(configName, "some_value")
			}
			testConfigValue(t, server, configName, testCase.expectedStatus)
		})
	}

	t.Run("authorized_not_marshallable", func(t *testing.T) {
		configName := "my.config.value"
		cfg, server := getConfigServer(t, authorizedSet{configName: {}})
		cfg.SetWithoutSource(configName, make(chan int))
		testConfigValue(t, server, "my.config.value", http.StatusInternalServerError)
	})
}

func getConfigServer(t *testing.T, authorizedConfigPaths map[string]struct{}) (*config.MockConfig, *httptest.Server) {
	t.Helper()

	cfg := config.Mock(t)
	configEndpointMux := GetConfigEndpointMux(cfg, authorizedConfigPaths, t.Name())
	server := httptest.NewServer(configEndpointMux)
	t.Cleanup(server.Close)

	return cfg, server
}
