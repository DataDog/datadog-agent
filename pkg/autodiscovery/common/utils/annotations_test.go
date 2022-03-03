// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCheckNames(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		adIdentifier string
		checkNames   []string
	}{
		{
			name:         "no check metadata",
			annotations:  map[string]string{},
			adIdentifier: "redis",
			checkNames:   []string{},
		},
		{
			name: "legacy annotations",
			annotations: map[string]string{
				"service-discovery.datadoghq.com/redis.check_names": `["redisdb"]`,
			},
			adIdentifier: "redis",
			checkNames:   []string{"redisdb"},
		},
		{
			name: "v1 annotations",
			annotations: map[string]string{
				"ad.datadoghq.com/redis.check_names":                `["redisdb"]`,
				"service-discovery.datadoghq.com/redis.check_names": `["foo"]`,
			},
			adIdentifier: "redis",
			checkNames:   []string{"redisdb"},
		},
		{
			name: "v2 annotations",
			annotations: map[string]string{
				"ad.datadoghq.com/redis.checks": `{
					"redisdb": {}
				}`,
				"service-discovery.datadoghq.com/redis.check_names": `["foo"]`,
				"ad.datadoghq.com/redis.check_names":                `["bar"]`,
			},
			adIdentifier: "redis",
			checkNames:   []string{"redisdb"},
		},
	}

	for _, tt := range tests {
		checkNames, err := ExtractCheckNames(tt.annotations, tt.adIdentifier)
		assert.Nil(t, err)
		assert.ElementsMatch(t, tt.checkNames, checkNames, "check names do not match")

	}
}

func TestExtractTemplatesFromPodAnnotations(t *testing.T) {
	const adID = "docker://foobar"

	tests := []struct {
		name         string
		annotations  map[string]string
		adIdentifier string
		output       []integration.Config
		errs         []error
	}{
		{
			name: "Nominal case with two templates",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  "[\"apache\",\"http_check\"]",
				"ad.datadoghq.com/foobar.init_configs": "[{},{}]",
				"ad.datadoghq.com/foobar.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"},{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}]",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
				{
					Name:          "http_check",
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Take one, ignore one",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  "[\"apache\"]",
				"ad.datadoghq.com/foobar.init_configs": "[{}]",
				"ad.datadoghq.com/foobar.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"quux.check_names":                     "[\"apache\"]",
				"quux.init_configs":                    "[{}]",
				"quux.instances":                       "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Logs config",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.logs": "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Check + logs",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  "[\"apache\"]",
				"ad.datadoghq.com/foobar.init_configs": "[{}]",
				"ad.datadoghq.com/foobar.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"ad.datadoghq.com/foobar.logs":         "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Missing check_names, silently ignore map",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.init_configs": "[{}]",
				"ad.datadoghq.com/foobar.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "foobar",
			output:       nil,
		},
		{
			name: "Missing init_configs, error out",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names": "[\"apache\"]",
				"ad.datadoghq.com/foobar.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "foobar",
			output:       nil,
			errs:         []error{errors.New("could not extract checks config: missing init_configs key")},
		},
		{
			name: "Invalid instances json",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  "[\"apache\"]",
				"ad.datadoghq.com/foobar.init_configs": "[{}]",
				"ad.datadoghq.com/foobar.instances":    "[{\"apache_status_url\" \"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "foobar",
			output:       nil,
			errs:         []error{errors.New("could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key")},
		},
		{
			name: "Invalid logs json",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.logs": "{\"service\":\"any_service\",\"source\":\"any_source\"}",
			},
			adIdentifier: "foobar",
			output:       nil,
			errs:         []error{errors.New("could not extract logs config: invalid format, expected an array, got: 'map[service:any_service source:any_source]'")},
		},
		{
			name: "Invalid checks but valid logs",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names": "[\"apache\"]",
				"ad.datadoghq.com/foobar.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"ad.datadoghq.com/foobar.logs":        "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "foobar",
			errs:         []error{errors.New("could not extract checks config: missing init_configs key")},
			output: []integration.Config{
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Invalid checks and invalid logs",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names": "[\"apache\"]",
				"ad.datadoghq.com/foobar.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"ad.datadoghq.com/foobar.logs":        "{\"service\":\"any_service\",\"source\":\"any_source\"}",
			},
			adIdentifier: "foobar",
			errs: []error{
				errors.New("could not extract checks config: missing init_configs key"),
				errors.New("could not extract logs config: invalid format, expected an array, got: 'map[service:any_service source:any_source]'"),
			},
			output: nil,
		},
		{
			name: "Two ways, same config (1)",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  `["apache","apache"]`,
				"ad.datadoghq.com/foobar.init_configs": `[{},{}]`,
				"ad.datadoghq.com/foobar.instances":    `[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto1"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "Two ways, same config (2)",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":  `["apache"]`,
				"ad.datadoghq.com/foobar.init_configs": `[{}]`,
				"ad.datadoghq.com/foobar.instances":    `[[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]]`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto1"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "v2 annotations",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"apache": {
						"instances": [
							{"apache_status_url":"http://%%host%%/server-status?auto2"}
						]
					}
				}`,
				"service-discovery.datadoghq.com/foobar.check_names": `["foo"]`,
				"ad.datadoghq.com/foobar.check_names":                `["bar"]`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "v2 annotations + logs",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"apache": {
						"instances": [
							{"apache_status_url":"http://%%host%%/server-status?auto2"}
						]
					}
				}`,
				"ad.datadoghq.com/foobar.logs": "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, errs := ExtractTemplatesFromPodAnnotations(adID, tt.annotations, tt.adIdentifier)
			assert.ElementsMatch(t, tt.output, configs)
			assert.ElementsMatch(t, tt.errs, errs)
		})
	}
}

func TestExtractTemplatesFromMap(t *testing.T) {
	for nb, tc := range []struct {
		source       map[string]string
		adIdentifier string
		prefix       string
		output       []integration.Config
		errs         []error
	}{
		{
			// Nominal case with two templates
			source: map[string]string{
				"prefix.check_names":  "[\"apache\",\"http_check\"]",
				"prefix.init_configs": "[{},{}]",
				"prefix.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"},{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
				{
					Name:          "http_check",
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Take one, ignore one
			source: map[string]string{
				"prefix.check_names":   "[\"apache\"]",
				"prefix.init_configs":  "[{}]",
				"prefix.instances":     "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"prefix2.check_names":  "[\"apache\"]",
				"prefix2.init_configs": "[{}]",
				"prefix2.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Logs config
			source: map[string]string{
				"prefix.logs": "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Check + logs
			source: map[string]string{
				"prefix.check_names":  "[\"apache\"]",
				"prefix.init_configs": "[{}]",
				"prefix.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"prefix.logs":         "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Missing check_names, silently ignore map
			source: map[string]string{
				"prefix.init_configs": "[{}]",
				"prefix.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       nil,
		},
		{
			// Missing init_configs, error out
			source: map[string]string{
				"prefix.check_names": "[\"apache\"]",
				"prefix.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       nil,
			errs:         []error{errors.New("could not extract checks config: missing init_configs key")},
		},
		{
			// Invalid instances json
			source: map[string]string{
				"prefix.check_names":  "[\"apache\"]",
				"prefix.init_configs": "[{}]",
				"prefix.instances":    "[{\"apache_status_url\" \"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       nil,
			errs:         []error{errors.New("could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key")},
		},
		{
			// Invalid logs json
			source: map[string]string{
				"prefix.logs": "{\"service\":\"any_service\",\"source\":\"any_source\"}",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       nil,
			errs:         []error{errors.New("could not extract logs config: invalid format, expected an array, got: ")},
		},
		{
			// Invalid checks but valid logs
			source: map[string]string{
				"prefix.check_names": "[\"apache\"]",
				"prefix.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"prefix.logs":        "[{\"service\":\"any_service\",\"source\":\"any_source\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			errs:         []error{errors.New("could not extract checks config: missing init_configs key")},
			output: []integration.Config{
				{
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Invalid checks and invalid logs
			source: map[string]string{
				"prefix.check_names": "[\"apache\"]",
				"prefix.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
				"prefix.logs":        "{\"service\":\"any_service\",\"source\":\"any_source\"}",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			errs: []error{
				errors.New("could not extract checks config: missing init_configs key"),
				errors.New("could not extract logs config: invalid format, expected an array, got: "),
			},
			output: nil,
		},
		{
			// Two ways, same config (1)
			source: map[string]string{
				"prefix.check_names":  `["apache","apache"]`,
				"prefix.init_configs": `[{},{}]`,
				"prefix.instances":    `[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]`,
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto1"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
		{
			// Two ways, same config (2)
			source: map[string]string{
				"prefix.check_names":  `["apache"]`,
				"prefix.init_configs": `[{}]`,
				"prefix.instances":    `[[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]]`,
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto1"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{"id"},
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.source), func(t *testing.T) {
			assert := assert.New(t)
			configs, errs := ExtractTemplatesFromMap(tc.adIdentifier, tc.source, tc.prefix)
			assert.EqualValues(tc.output, configs)

			if len(tc.errs) == 0 {
				assert.Equal(0, len(errs))
			} else {
				for i, err := range errs {
					assert.NotNil(err)
					assert.Contains(err.Error(), tc.errs[i].Error())
				}
			}
		})
	}
}

func TestGetCustomCheckID(t *testing.T) {
	tests := []struct {
		name          string
		annotations   map[string]string
		containerName string
		want          string
		found         bool
	}{
		{
			name:          "found",
			annotations:   map[string]string{"ad.datadoghq.com/foo.check.id": "bar"},
			containerName: "foo",
			want:          "bar",
			found:         true,
		},
		{
			name:          "not found",
			annotations:   map[string]string{"ad.datadoghq.com/foo.check.id": "bar"},
			containerName: "baz",
			want:          "",
			found:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetCustomCheckID(tt.annotations, tt.containerName)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.found, found)
		})
	}
}

func TestParseJSONValue(t *testing.T) {
	tests := []struct {
		name                string
		inputValue          string
		expectedReturnValue [][]integration.Data
		expectedErr         error
	}{
		{
			name:                "empty value",
			inputValue:          "",
			expectedErr:         fmt.Errorf("Value is empty"),
			expectedReturnValue: nil,
		},
		{
			name:                "value is not a list",
			inputValue:          "{}",
			expectedErr:         fmt.Errorf("failed to unmarshal JSON: json: cannot unmarshal object into Go value of type []interface {}"),
			expectedReturnValue: nil,
		},
		{
			name:                "invalid json",
			inputValue:          "[{]",
			expectedErr:         fmt.Errorf("failed to unmarshal JSON: invalid character ']' looking for beginning of object key string"),
			expectedReturnValue: nil,
		},
		{
			name:                "bad type",
			inputValue:          "[1, {\"test\": 1}, \"test\"]",
			expectedErr:         fmt.Errorf("failed to decode JSON Object '1' to integration.Data struct: found non JSON object type, value is: '1'"),
			expectedReturnValue: nil,
		},
		{
			name:        "valid input",
			inputValue:  "[{\"test\": 1}, {\"test\": 2}]",
			expectedErr: nil,
			expectedReturnValue: [][]integration.Data{
				{integration.Data("{\"test\":1}")},
				{integration.Data("{\"test\":2}")},
			},
		},
		{
			name:        "valid input with list",
			inputValue:  "[[{\"test\": 1.1},{\"test\": 1.2}], {\"test\": 2}]",
			expectedErr: nil,
			expectedReturnValue: [][]integration.Data{
				{integration.Data("{\"test\":1.1}"), integration.Data("{\"test\":1.2}")},
				{integration.Data("{\"test\":2}")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONValue(tt.inputValue)
			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, got, tt.expectedReturnValue)
		})
	}
}

func TestParseCheckNames(t *testing.T) {
	// empty value
	res, err := ParseCheckNames("")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// value is not a list
	res, err = ParseCheckNames("{}")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// invalid json
	res, err = ParseCheckNames("[{]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// ignore bad type
	res, err = ParseCheckNames("[1, {\"test\": 1}, \"test\"]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// valid input
	res, err = ParseCheckNames("[\"test1\", \"test2\"]")
	assert.Nil(t, err)
	assert.NotNil(t, res)
	require.Len(t, res, 2)
	assert.Equal(t, []string{"test1", "test2"}, res)
}

func TestBuildTemplates(t *testing.T) {
	key := "id"
	tests := []struct {
		name            string
		inputCheckNames []string
		inputInitConfig [][]integration.Data
		inputInstances  [][]integration.Data
		expectedConfigs []integration.Config
	}{
		{
			name:            "wrong number of checkNames",
			inputCheckNames: []string{"a", "b"},
			inputInitConfig: [][]integration.Data{{integration.Data("")}},
			inputInstances:  [][]integration.Data{{integration.Data("")}},
			expectedConfigs: []integration.Config{},
		},
		{
			name:            "valid inputs",
			inputCheckNames: []string{"a", "b"},
			inputInitConfig: [][]integration.Data{{integration.Data("{\"test\": 1}")}, {integration.Data("{}")}},
			inputInstances:  [][]integration.Data{{integration.Data("{}")}, {integration.Data("{1:2}")}},
			expectedConfigs: []integration.Config{
				{
					Name:          "a",
					ADIdentifiers: []string{key},
					InitConfig:    integration.Data("{\"test\": 1}"),
					Instances:     []integration.Data{integration.Data("{}")},
				},
				{
					Name:          "b",
					ADIdentifiers: []string{key},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{1:2}")},
				},
			},
		},
		{
			name:            "valid inputs with list",
			inputCheckNames: []string{"a", "b"},
			inputInitConfig: [][]integration.Data{{integration.Data("{\"test\": 1}")}, {integration.Data("{}")}},
			inputInstances:  [][]integration.Data{{integration.Data("{\"foo\": 1}"), integration.Data("{\"foo\": 2}")}, {integration.Data("{1:2}")}},
			expectedConfigs: []integration.Config{
				{
					Name:          "a",
					ADIdentifiers: []string{key},
					InitConfig:    integration.Data("{\"test\": 1}"),
					Instances:     []integration.Data{integration.Data("{\"foo\": 1}")},
				},
				{
					Name:          "a",
					ADIdentifiers: []string{key},
					InitConfig:    integration.Data("{\"test\": 1}"),
					Instances:     []integration.Data{integration.Data("{\"foo\": 2}")},
				},
				{
					Name:          "b",
					ADIdentifiers: []string{key},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{1:2}")},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedConfigs, BuildTemplates(key, tt.inputCheckNames, tt.inputInitConfig, tt.inputInstances))
		})
	}
}
