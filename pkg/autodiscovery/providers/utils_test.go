// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// testing helper function to retrieve configuration from a list of configs
func getConfigByName(configs []integration.Config, name string, adIdentifiers []string) *integration.Config {
	for _, config := range configs {
		if config.Name == name {
			match := true
			for i, identifier := range config.ADIdentifiers {
				if identifier != adIdentifiers[i] {
					match = false
					break
				}
			}
			if match {
				return &config
			}
		}
	}

	return nil
}

func TestParseJSONValue(t *testing.T) {
	// empty value
	res, err := parseJSONValue("")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// value is not a list
	res, err = parseJSONValue("{}")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// invalid json
	res, err = parseJSONValue("[{]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// bad type
	res, err = parseJSONValue("[1, {\"test\": 1}, \"test\"]")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "found non JSON object type, value is: '1'", err.Error())

	// valid input
	res, err = parseJSONValue("[{\"test\": 1}, {\"test\": 2}]")
	assert.Nil(t, err)
	assert.NotNil(t, res)
	require.Len(t, res, 2)
	assert.JSONEq(t, string(integration.Data("{\"test\":1}")), string(res[0]))
	assert.JSONEq(t, string(integration.Data("{\"test\":2}")), string(res[1]))
}

func TestParseCheckNames(t *testing.T) {
	// empty value
	res, err := parseCheckNames("")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// value is not a list
	res, err = parseCheckNames("{}")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// invalid json
	res, err = parseCheckNames("[{]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// ignore bad type
	res, err = parseCheckNames("[1, {\"test\": 1}, \"test\"]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// valid input
	res, err = parseCheckNames("[\"test1\", \"test2\"]")
	assert.Nil(t, err)
	assert.NotNil(t, res)
	require.Len(t, res, 2)
	assert.Equal(t, []string{"test1", "test2"}, res)
}

func TestBuildStoreKey(t *testing.T) {
	res := buildStoreKey()
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("")
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("foo")
	assert.Equal(t, "/datadog/check_configs/foo", res)
	res = buildStoreKey("foo", "bar")
	assert.Equal(t, "/datadog/check_configs/foo/bar", res)
	res = buildStoreKey("foo", "bar", "bazz")
	assert.Equal(t, "/datadog/check_configs/foo/bar/bazz", res)
}

func TestBuildTemplates(t *testing.T) {
	// wrong number of checkNames
	res := buildTemplates("id",
		[]string{"a", "b"},
		[]integration.Data{integration.Data("")},
		[]integration.Data{integration.Data("")})
	assert.Len(t, res, 0)

	res = buildTemplates("id",
		[]string{"a", "b"},
		[]integration.Data{integration.Data("{\"test\": 1}"), integration.Data("{}")},
		[]integration.Data{integration.Data("{}"), integration.Data("{1:2}")})
	require.Len(t, res, 2)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "id", res[0].ADIdentifiers[0])
	assert.Equal(t, res[0].Name, "a")
	assert.JSONEq(t, string(res[0].InitConfig), string(integration.Data("{\"test\": 1}")))
	assert.Equal(t, res[0].Instances, []integration.Data{integration.Data("{}")})

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "id", res[1].ADIdentifiers[0])
	assert.Equal(t, res[1].Name, "b")
	assert.JSONEq(t, string(res[1].InitConfig), string(integration.Data("{}")))
	assert.Equal(t, res[1].Instances, []integration.Data{integration.Data("{1:2}")})
}

func TestExtractTemplatesFromMap(t *testing.T) {
	for nb, tc := range []struct {
		source       map[string]string
		adIdentifier string
		prefix       string
		output       []integration.Config
		err          error
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
			// Missing check_names, silently ignore map
			source: map[string]string{
				"prefix.init_configs": "[{}]",
				"prefix.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       []integration.Config{},
		},
		{
			// Missing init_configs, error out
			source: map[string]string{
				"prefix.check_names": "[\"apache\"]",
				"prefix.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			adIdentifier: "id",
			prefix:       "prefix.",
			output:       []integration.Config{},
			err:          errors.New("missing init_configs key"),
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
			output:       []integration.Config{},
			err:          errors.New("in instances: Failed to unmarshal JSON"),
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.source), func(t *testing.T) {
			assert := assert.New(t)
			configs, err := extractTemplatesFromMap(tc.adIdentifier, tc.source, tc.prefix)

			for _, config := range configs {
				expectedCfg := getConfigByName(tc.output, config.Name, []string{tc.adIdentifier})
				require.NotNil(t, expectedCfg)

				assert.Equal(expectedCfg.Name, config.Name)
				assert.EqualValues(expectedCfg.ADIdentifiers, []string{tc.adIdentifier})
				assert.JSONEq(string(expectedCfg.InitConfig), string(config.InitConfig))

				// NOTE: this could break if Instances has a different order
				for jdx, instance := range config.Instances {
					assert.JSONEq(string(expectedCfg.Instances[jdx]), string(instance))
				}
			}

			if tc.err == nil {
				assert.Nil(err)
			} else {
				assert.NotNil(err)
				assert.Contains(err.Error(), tc.err.Error())
			}
		})
	}
}
