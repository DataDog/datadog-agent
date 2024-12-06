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

func TestExtractCheckNamesFromAnnotations(t *testing.T) {
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
		{
			name: "v2 annotations, multiple checks",
			annotations: map[string]string{
				"ad.datadoghq.com/redis.checks": `{
					"redisdb": {},
					"foobar": {}
				}`,
			},
			adIdentifier: "redis",
			checkNames:   []string{"redisdb", "foobar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkNames, err := ExtractCheckNamesFromPodAnnotations(tt.annotations, tt.adIdentifier)
			assert.Nil(t, err)
			assert.ElementsMatch(t, tt.checkNames, checkNames, "check names do not match")
		})
	}
}

func TestExtractTemplatesFromAnnotations(t *testing.T) {
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
			name: "Nominal case with two templates and ignore autodiscovery tags",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":               "[\"apache\",\"http_check\"]",
				"ad.datadoghq.com/foobar.init_configs":              "[{},{}]",
				"ad.datadoghq.com/foobar.instances":                 "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"},{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}]",
				"ad.datadoghq.com/foobar.ignore_autodiscovery_tags": "true",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:                    "apache",
					Instances:               []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:              integration.Data("{}"),
					ADIdentifiers:           []string{adID},
					IgnoreAutodiscoveryTags: true,
				},
				{
					Name:                    "http_check",
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					InitConfig:              integration.Data("{}"),
					ADIdentifiers:           []string{adID},
					IgnoreAutodiscoveryTags: true,
				},
			},
		},
		{
			name: "Nominal case with two templates and check tag cardinality",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.check_names":           "[\"apache\",\"http_check\"]",
				"ad.datadoghq.com/foobar.init_configs":          "[{},{}]",
				"ad.datadoghq.com/foobar.instances":             "[{\"apache_status_url\":\"http://%%host%%/server-status?auto\"},{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}]",
				"ad.datadoghq.com/foobar.check_tag_cardinality": "low",
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:                "apache",
					Instances:           []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					InitConfig:          integration.Data("{}"),
					ADIdentifiers:       []string{adID},
					CheckTagCardinality: "low",
				},
				{
					Name:                "http_check",
					Instances:           []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					InitConfig:          integration.Data("{}"),
					ADIdentifiers:       []string{adID},
					CheckTagCardinality: "low",
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
					Name:          "apache",
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
			name: "v2 annotations with ignore_ad_tags",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"apache": {
						"instances": [
							{"apache_status_url":"http://%%host%%/server-status?auto2"}
						],
						"ignore_autodiscovery_tags": true
					}
				}`,
				"service-discovery.datadoghq.com/foobar.check_names": `["foo"]`,
				"ad.datadoghq.com/foobar.check_names":                `["bar"]`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:                    "apache",
					Instances:               []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:              integration.Data("{}"),
					ADIdentifiers:           []string{adID},
					IgnoreAutodiscoveryTags: true,
				},
			},
		},
		{
			name: "v2 annotations with adv1 ignore_ad_tags",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"apache": {
						"instances": [
							{"apache_status_url":"http://%%host%%/server-status?auto2"}
						]
					}
				}`,
				"ad.datadoghq.com/foobar.ignore_autodiscovery_tags":  "true",
				"service-discovery.datadoghq.com/foobar.check_names": `["foo"]`,
				"ad.datadoghq.com/foobar.check_names":                `["bar"]`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:                    "apache",
					Instances:               []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto2"}`)},
					InitConfig:              integration.Data("{}"),
					ADIdentifiers:           []string{adID},
					IgnoreAutodiscoveryTags: false,
				},
			},
		},
		{
			name: "v2 annotations with init_config",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"jmx": {
						"init_config": {"is_jmx": true, "collect_default_metrics": false},
						"instances": [{"host":"%%host%%","port":"9012"}]
					}
				}`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "jmx",
					Instances:     []integration.Data{integration.Data(`{"host":"%%host%%","port":"9012"}`)},
					InitConfig:    integration.Data(`{"is_jmx": true, "collect_default_metrics": false}`),
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
					Name:          "apache",
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
				},
			},
		},
		{
			name: "v2 annotations label logs",
			annotations: map[string]string{
				"ad.datadoghq.com/foobar.checks": `{
					"apache": {
						"logs": [{"service":"any_service","source":"any_source"}]
					}
				}`,
			},
			adIdentifier: "foobar",
			output: []integration.Config{
				{
					Name:          "apache",
					LogsConfig:    integration.Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
					ADIdentifiers: []string{adID},
					InitConfig:    integration.Data("{}"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, errs := ExtractTemplatesFromAnnotations(adID, tt.annotations, tt.adIdentifier)
			assert.ElementsMatch(t, tt.output, configs)
			assert.ElementsMatch(t, tt.errs, errs)
		})
	}
}

func TestExtractCheckIDFromPodAnnotations(t *testing.T) {
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
			got, found := ExtractCheckIDFromPodAnnotations(tt.annotations, tt.containerName)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.found, found)
		})
	}
}
