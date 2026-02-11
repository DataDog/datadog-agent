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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	model "github.com/DataDog/datadog-agent/pkg/config/model"
)

type testCase struct {
	name           string
	existing       bool
	expectedStatus int
}

type prefixTestCase struct {
	name           string
	configName     string
	expectedStatus int
}

type expvals struct {
	Success map[string]int `json:"success"`
	Errors  map[string]int `json:"errors"`
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

	// roundtrip our existing config value so that we emulate how values get serialized when we
	// write them out in the HTTP response in the first place: if we don't do this, then we
	// potentially end up with test failures purely due to property type mismatches, even when the
	// data is exactly the same
	existing := configEndpoint.cfg.Get(configName)
	existingBody, err := json.Marshal(existing)
	require.NoError(t, err)

	var existingValue interface{}
	err = json.Unmarshal(existingBody, &existingValue)
	require.NoError(t, err)

	var configValue interface{}
	err = json.Unmarshal(body, &configValue)
	require.NoError(t, err)
	require.EqualValues(t, existingValue, configValue)
}

func TestConfigEndpoint(t *testing.T) {
	for _, testCase := range []testCase{
		{"existing_config", true, http.StatusOK},
		{"missing_config", false, http.StatusNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configName := "my.config.value"
			cfg, server, configEndpoint := getConfigServer(t)
			if testCase.existing {
				cfg.SetWithoutSource(configName, "some_value")
				cfg.SetKnown(configName) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
			}
			testConfigValue(t, configEndpoint, server, configName, testCase.expectedStatus)
		})
	}

	t.Run("not_marshallable", func(t *testing.T) {
		configName := "my.config.value"
		cfg, _, _ := getConfigServer(t)
		cfg.SetKnown(configName) //nolint:forbidigo // testing behavior
		// calling SetWithoutSource with an invalid type of data will panic
		assert.Panics(t, func() { cfg.SetWithoutSource(configName, make(chan int)) })
	})

	parentConfigName := "root.parent"
	childConfigNameOne := parentConfigName + ".child1"
	childConfigNameTwo := parentConfigName + ".child2"
	for _, testCase := range []prefixTestCase{
		{"nested_root", parentConfigName, http.StatusOK},
		{"nested_child_one", childConfigNameOne, http.StatusOK},
		{"nested_child_two", childConfigNameTwo, http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg, server, configEndpoint := getConfigServer(t)
			cfg.SetWithoutSource(childConfigNameOne, "child1_value")
			cfg.SetKnown(childConfigNameOne) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
			cfg.SetWithoutSource(childConfigNameTwo, "child2_value")
			cfg.SetKnown(childConfigNameTwo) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

			testConfigValue(t, configEndpoint, server, testCase.configName, testCase.expectedStatus)
		})
	}

	t.Run("unknown_path_returns_404", func(t *testing.T) {
		cfg, server, configEndpoint := getConfigServer(t)
		cfg.SetWithoutSource("known.key", "value")
		cfg.SetKnown("known.key") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

		testConfigValue(t, configEndpoint, server, "known.key", http.StatusOK)
		testConfigValue(t, configEndpoint, server, "unknown.path", http.StatusNotFound)
	})
}

func TestConfigListEndpoint(t *testing.T) {
	testCases := []struct {
		name         string
		configValues map[string]interface{}
	}{
		{"single_config", map[string]interface{}{"my.config.value": "some_value"}},
		{"multiple_configs", map[string]interface{}{"my.config.value": "some_value", "my.other.config.value": 12.5}},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			cfg, server, _ := getConfigServer(t)
			for key, value := range test.configValues {
				cfg.SetWithoutSource(key, value)
				cfg.SetKnown(key) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
			}

			// test with and without trailing slash
			for _, urlSuffix := range []string{"", "/"} {
				resp, err := server.Client().Get(server.URL + urlSuffix)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)

				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var configValues map[string]interface{}
				err = json.Unmarshal(data, &configValues)
				require.NoError(t, err)

				// Response includes all config keys; verify our test keys are present with correct values
				for key, expectedVal := range test.configValues {
					require.Contains(t, configValues, key, "response should contain key %q", key)
					// Roundtrip expected through JSON to match response types (e.g. int -> float64)
					expectedBody, _ := json.Marshal(expectedVal)
					var expectedRoundtrip interface{}
					_ = json.Unmarshal(expectedBody, &expectedRoundtrip)
					assert.EqualValues(t, expectedRoundtrip, configValues[key], "key %q", key)
				}
			}
		})
	}
}

func TestConfigEndpointJSONError(t *testing.T) {
	// This test validate that the API can serialized map[interface{}]interface{} to JSON (ie: validate that we're
	// using github.com/json-iterator/go rather than encoding/json)

	cfg, server, _ := getConfigServer(t)
	cfg.SetWithoutSource("my.config.value", []interface{}{
		map[interface{}]interface{}{"a": "b", "c": "d"},
	})
	cfg.SetKnown("my.config.value") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	for _, endpoint := range []string{"/", "/my.config"} {
		resp, err := server.Client().Get(server.URL + endpoint)
		require.NoError(t, err)

		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var configValues map[string]interface{}
		err = json.Unmarshal(data, &configValues)
		require.NoError(t, err)
	}
}

func checkExpvars(t *testing.T, beforeVars, afterVars expvals, configName string, expectedStatus int) {
	t.Helper()

	switch expectedStatus {
	case http.StatusOK:
		beforeVars.Success[configName]++
	case http.StatusNotFound, http.StatusInternalServerError:
		beforeVars.Errors[configName]++
	default:
		t.Fatalf("unexpected status: %d", expectedStatus)
	}

	require.EqualValues(t, beforeVars, afterVars)
}

func getConfigServer(t *testing.T) (model.Config, *httptest.Server, *configEndpoint) {
	t.Helper()

	cfg := configmock.New(t)
	configEndpointMux, configEndpoint := getConfigEndpoint(cfg, t.Name())
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
