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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type testCase struct {
	name           string
	authorized     bool
	existing       bool
	expectedStatus int
}

type nestedTestCase struct {
	name           string
	hasDefault     bool
	override       bool
	expectedStatus int
}

type prefixTestCase struct {
	name           string
	configName     string
	expectedStatus int
}

type expvals struct {
	Success      map[string]int `json:"success"`
	Errors       map[string]int `json:"errors"`
	Unauthorized map[string]int `json:"unauthorized"`
}

func testConfigValue(t *testing.T, configEndpoint *configEndpoint, server *httptest.Server, configName string, expectedStatus int) {
	t.Helper()

	beforeVars := getExpvals(t, configEndpoint)
	resp, err := server.Client().Get(server.URL + "/" + configName)
	require.NoError(t, err)
	defer resp.Body.Close()

	afterVars := getExpvals(t, configEndpoint)
	checkExpvars(t, beforeVars, afterVars, configName, expectedStatus)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, expectedStatus, resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return
	}

	var configValue interface{}
	err = json.Unmarshal(body, &configValue)
	require.NoError(t, err)
	require.EqualValues(t, configEndpoint.cfg.Get(configName), configValue)
}

func TestConfigEndpoint(t *testing.T) {
	t.Run("core_config", func(t *testing.T) {
		_, server, configEndpoint := getConfigServer(t, authorizedConfigPathsCore)
		for configName := range authorizedConfigPathsCore {
			testConfigValue(t, configEndpoint, server, configName, http.StatusOK)
		}
	})

	for _, testCase := range []testCase{
		{"authorized_existing_config", true, true, http.StatusOK},
		{"authorized_missing_config", true, false, http.StatusOK},
		{"unauthorized_existing_config", false, true, http.StatusForbidden},
		{"unauthorized_missing_config", false, false, http.StatusForbidden},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configName := "my.config.value"
			authorizedConfigPaths := authorizedSet{}
			if testCase.authorized {
				authorizedConfigPaths[configName] = struct{}{}
			}
			cfg, server, configEndpoint := getConfigServer(t, authorizedConfigPaths)
			if testCase.existing {
				cfg.SetWithoutSource(configName, "some_value")
			}
			testConfigValue(t, configEndpoint, server, configName, testCase.expectedStatus)
		})
	}

	t.Run("authorized_not_marshallable", func(t *testing.T) {
		configName := "my.config.value"
		cfg, server, configEndpoint := getConfigServer(t, authorizedSet{configName: {}})
		cfg.SetWithoutSource(configName, make(chan int))
		testConfigValue(t, configEndpoint, server, configName, http.StatusInternalServerError)
	})

	for _, testCase := range []nestedTestCase{
		{"authorized_nested_no_default_no_override", false, false, http.StatusNotFound},
		{"authorized_nested_no_default_override", false, true, http.StatusOK},
		{"authorized_nested_default_no_override", true, false, http.StatusOK},
		{"authorized_nested_default_override", true, true, http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configName := "my.config.value"

			cfg, server, configEndpoint := getConfigServer(t, authorizedSet{configName: struct{}{}})
			if testCase.hasDefault {
				cfg.Set(configName, "default_value", model.SourceDefault)
			}
			if testCase.override {
				cfg.Set(configName, "override_value", model.SourceEnvVar)
			}
			testConfigValue(t, configEndpoint, server, configName, testCase.expectedStatus)
		})
	}

	parentConfigName := "root.parent"
	childConfigNameOne := parentConfigName + ".child1"
	childConfigNameTwo := parentConfigName + ".child2"
	for _, testCase := range []prefixTestCase{
		{"authorized_nested_prefix_path_rule_root", parentConfigName, http.StatusOK},
		{"authorized_nested_prefix_path_rule_root", childConfigNameOne, http.StatusOK},
		{"authorized_nested_prefix_path_rule_root", childConfigNameTwo, http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			parentConfigPath := parentConfigName + "."

			cfg, server, configEndpoint := getConfigServer(t, authorizedSet{parentConfigPath: struct{}{}})

			cfg.SetWithoutSource(childConfigNameOne, "child1_value")
			cfg.SetWithoutSource(childConfigNameTwo, "child2_value")

			testConfigValue(t, configEndpoint, server, testCase.configName, testCase.expectedStatus)
		})
	}

	t.Run("unauthorized_nested_prefix_path_rule", func(t *testing.T) {
		parentConfigName := "root.parent"
		childConfigName := parentConfigName + ".child"

		cfg, server, configEndpoint := getConfigServer(t, authorizedSet{childConfigName: struct{}{}})

		cfg.SetWithoutSource(childConfigName, "child_value")

		testConfigValue(t, configEndpoint, server, childConfigName, http.StatusOK)
		testConfigValue(t, configEndpoint, server, parentConfigName, http.StatusForbidden)
	})
}

func TestConfigListEndpoint(t *testing.T) {
	testCases := []struct {
		name              string
		configValues      map[string]interface{}
		authorizedConfigs authorizedSet
	}{
		{
			"empty_config",
			map[string]interface{}{"some.config": "some_value"},
			authorizedSet{},
		},
		{
			"single_config",
			map[string]interface{}{"some.config": "some_value", "my.config.value": "some_value"},
			authorizedSet{"my.config.value": {}},
		},
		{
			"multiple_configs",
			map[string]interface{}{"my.config.value": "some_value", "my.other.config.value": 12.5},
			authorizedSet{"my.config.value": {}, "my.other.config.value": {}},
		},
		{
			"missing_config",
			map[string]interface{}{"my.config.value": "some_value"},
			authorizedSet{"my.config.value": {}, "my.other.config.value": {}},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			cfg, server, _ := getConfigServer(t, test.authorizedConfigs)
			for key, value := range test.configValues {
				cfg.SetWithoutSource(key, value)
			}

			// test with and without trailing slash
			for _, urlSuffix := range []string{"", "/"} {
				resp, err := server.Client().Get(server.URL + urlSuffix)
				require.NoError(t, err)
				defer resp.Body.Close()

				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var configValues map[string]interface{}
				err = json.Unmarshal(data, &configValues)
				require.NoError(t, err)

				expectedValues := make(map[string]interface{})
				for key := range test.authorizedConfigs {
					expectedValues[key] = cfg.Get(key)
				}

				assert.Equal(t, expectedValues, configValues)
			}
		})
	}

}

func checkExpvars(t *testing.T, beforeVars, afterVars expvals, configName string, expectedStatus int) {
	t.Helper()

	switch expectedStatus {
	case http.StatusOK:
		beforeVars.Success[configName]++
	case http.StatusForbidden:
		beforeVars.Unauthorized[configName]++
	case http.StatusInternalServerError:
		beforeVars.Errors[configName]++
	default:
		t.Fatalf("unexpected status: %d", expectedStatus)
	}

	require.EqualValues(t, beforeVars, afterVars)
}

func getConfigServer(t *testing.T, authorizedConfigPaths map[string]struct{}) (*config.MockConfig, *httptest.Server, *configEndpoint) {
	t.Helper()

	cfg := config.Mock(t)
	configEndpointMux, configEndpoint := getConfigEndpoint(cfg, authorizedConfigPaths, t.Name())
	server := httptest.NewServer(configEndpointMux)
	t.Cleanup(server.Close)

	return cfg, server, configEndpoint
}

func getExpvals(t *testing.T, configEndpoint *configEndpoint) expvals {
	t.Helper()

	vars := expvals{}
	// error on unknown fields
	dec := json.NewDecoder(strings.NewReader(configEndpoint.expvars.String()))
	dec.DisallowUnknownFields()
	err := dec.Decode(&vars)
	require.NoError(t, err)
	require.False(t, dec.More())

	return vars
}
