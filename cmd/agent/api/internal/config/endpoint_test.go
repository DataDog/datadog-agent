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
	t.Run("real allowed default", func(t *testing.T) {
		cfg, server := getConfigServer(t, authorizedConfigPaths)
		for configName := range authorizedConfigPaths {
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
		{true, true, http.StatusOK}, {true, false, http.StatusNotFound},
		{false, true, http.StatusForbidden}, {false, false, http.StatusForbidden},
	} {
		testName := getTestCaseName(testCase.authorized, testCase.existing)
		t.Run(testName, func(t *testing.T) {
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

	t.Run("authorized not marshallable", func(t *testing.T) {
		configName := "my.config.value"
		cfg, server := getConfigServer(t, authorizedSet{configName: {}})
		cfg.SetWithoutSource(configName, make(chan int))
		testConfigValue(t, server, "my.config.value", http.StatusInternalServerError)
	})
}

func ternary(condition bool, trueValue, falseValue string) string {
	if condition {
		return trueValue
	}
	return falseValue
}

func getTestCaseName(authorized, existing bool) string {
	auth := ternary(authorized, "authorized", "unauthorized")
	exist := ternary(existing, "existing", "missing")
	return auth + " " + exist
}

func getConfigServer(t *testing.T, authorizedConfigPaths map[string]struct{}) (*config.MockConfig, *httptest.Server) {
	t.Helper()

	cfg := config.Mock(t)
	configEndpointMux := getConfigEndpointMux(cfg, authorizedConfigPaths)
	server := httptest.NewServer(configEndpointMux)
	t.Cleanup(server.Close)

	return cfg, server
}
