// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestExtractCheckNamesFromContainerLabels(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]string
		checkNames []string
	}{
		{
			name:       "no check metadata",
			labels:     map[string]string{},
			checkNames: []string{},
		},
		{
			name: "v1 annotations",
			labels: map[string]string{
				"com.datadoghq.ad.check_names": `["redisdb"]`,
			},
			checkNames: []string{"redisdb"},
		},
		{
			name: "v2 annotations",
			labels: map[string]string{
				"com.datadoghq.ad.checks": `{
					"redisdb": {}
				}`,
				"com.datadoghq.ad.check_names": `["foo"]`,
			},
			checkNames: []string{"redisdb"},
		},
		{
			name: "v2 annotations, multiple checks",
			labels: map[string]string{
				"com.datadoghq.ad.checks": `{
					"redisdb": {},
					"foobar": {}
				}`,
			},
			checkNames: []string{"redisdb", "foobar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkNames, err := ExtractCheckNamesFromContainerLabels(tt.labels)
			assert.Nil(t, err)
			assert.ElementsMatch(t, tt.checkNames, checkNames, "check names do not match")
		})

	}
}

func TestExtractTemplatesFromContainerLabels(t *testing.T) {
	const adID = "docker://foobar"

	tests := []struct {
		name        string
		annotations map[string]string
		output      []integration.Config
		errs        []error
	}{
		{
			name: "Nominal case with two templates",
			annotations: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\",\"http_check\"]",
				"com.datadoghq.ad.init_configs": "[{},{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"},{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}]",
			},
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
			name: "Missing check_names, silently ignore map",
			annotations: map[string]string{
				"com.datadoghq.ad.init_configs": "[{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			output: nil,
		},
		{
			name: "Missing init_configs, error out",
			annotations: map[string]string{
				"com.datadoghq.ad.check_names": "[\"apache\"]",
				"com.datadoghq.ad.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			output: nil,
			errs:   []error{errors.New("could not extract checks config: missing init_configs key")},
		},
		{
			name: "Invalid instances json",
			annotations: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\"]",
				"com.datadoghq.ad.init_configs": "[{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\" \"http://%%host%%/server-status?auto\"}]",
			},
			output: nil,
			errs:   []error{errors.New("could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key")},
		},
		{
			name: "Invalid checks",
			annotations: map[string]string{
				"com.datadoghq.ad.check_names": "[\"apache\"]",
				"com.datadoghq.ad.instances":   "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}]",
			},
			errs:   []error{errors.New("could not extract checks config: missing init_configs key")},
			output: nil,
		},
		{
			name: "Two ways, same config (1)",
			annotations: map[string]string{
				"com.datadoghq.ad.check_names":  `["apache","apache"]`,
				"com.datadoghq.ad.init_configs": `[{},{}]`,
				"com.datadoghq.ad.instances":    `[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]`,
			},
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
				"com.datadoghq.ad.check_names":  `["apache"]`,
				"com.datadoghq.ad.init_configs": `[{}]`,
				"com.datadoghq.ad.instances":    `[[{"apache_status_url":"http://%%host%%/server-status?auto1"},{"apache_status_url":"http://%%host%%/server-status?auto2"}]]`,
			},
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
				"com.datadoghq.ad.checks": `{
					"apache": {
						"instances": [
							{"apache_status_url":"http://%%host%%/server-status?auto2"}
						]
					}
				}`,
				"com.datadoghq.ad.check_names": `["foo"]`,
			},
			output: []integration.Config{
				{
					Name:          "apache",
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:    integration.Data("{}"),
					ADIdentifiers: []string{adID},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, errs := ExtractTemplatesFromContainerLabels(adID, tt.annotations)
			assert.ElementsMatch(t, tt.output, configs)
			assert.ElementsMatch(t, tt.errs, errs)
		})
	}
}
