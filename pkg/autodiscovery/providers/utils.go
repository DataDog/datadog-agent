// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
	logsConfigPath string = "logs"
)

func init() {
	// Where to look for check templates if no custom path is defined
	config.Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	// Defaut Timeout in second when talking to storage for configuration (etcd, zookeeper, ...)
	config.Datadog.SetDefault("autoconf_template_url_timeout", 5)
}

// parseJSONValue returns a slice of slice of ConfigData parsed from the JSON
// contained in the `value` parameter
func parseJSONValue(value string) ([][]integration.Data, error) {
	if value == "" {
		return nil, fmt.Errorf("Value is empty")
	}

	var rawRes []interface{}
	var result [][]integration.Data

	err := json.Unmarshal([]byte(value), &rawRes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	for _, r := range rawRes {
		switch r.(type) {
		case []interface{}:
			objs := r.([]interface{})
			var subResult []integration.Data
			for idx := range objs {
				var init integration.Data
				init, err = parseJSONObjToData(objs[idx])
				if err != nil {
					return nil, fmt.Errorf("failed to decode JSON Object '%v' to integration.Data struct: %v", objs[idx], err)
				}
				subResult = append(subResult, init)
			}
			if subResult != nil {
				result = append(result, subResult)
			}
		default:
			var init integration.Data
			init, err = parseJSONObjToData(r)
			if err != nil {
				return nil, fmt.Errorf("failed to decode JSON Object '%v' to integration.Data struct: %v", r, err)
			}
			result = append(result, []integration.Data{init})
		}
	}
	return result, nil
}

func parseJSONObjToData(r interface{}) (integration.Data, error) {
	switch r.(type) {
	case map[string]interface{}:
		return json.Marshal(r)
	default:
		return nil, fmt.Errorf("found non JSON object type, value is: '%v'", r)
	}
}

func parseCheckNames(names string) (res []string, err error) {
	if names == "" {
		return nil, fmt.Errorf("check_names is empty")
	}

	if err = json.Unmarshal([]byte(names), &res); err != nil {
		return nil, err
	}

	return res, nil
}

func buildStoreKey(key ...string) string {
	parts := []string{config.Datadog.GetString("autoconf_template_dir")}
	parts = append(parts, key...)
	return path.Join(parts...)
}

func buildTemplates(key string, checkNames []string, initConfigs, instances [][]integration.Data) []integration.Config {
	templates := make([]integration.Config, 0)

	// sanity checks
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length, not using them.")
		return templates
	}
	for idx := range initConfigs {
		if len(initConfigs[idx]) != 1 {
			log.Error("Templates init Configs list is not valid, not using Templates entries")
			return templates
		}
	}

	for idx := range checkNames {
		for _, instance := range instances[idx] {
			templates = append(templates, integration.Config{
				Name:          checkNames[idx],
				InitConfig:    integration.Data(initConfigs[idx][0]),
				Instances:     []integration.Data{instance},
				ADIdentifiers: []string{key},
			})
		}
	}
	return templates
}

// extractTemplatesFromMap looks for autodiscovery configurations in a given map
// (either docker labels or kubernetes annotations) and returns them if found.
func extractTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, []error) {
	var configs []integration.Config
	var errors []error

	checksConfigs, err := extractCheckTemplatesFromMap(key, input, prefix)
	if err != nil {
		errors = append(errors, fmt.Errorf("could not extract checks config: %v", err))
	}
	configs = append(configs, checksConfigs...)

	logsConfigs, err := extractLogsTemplatesFromMap(key, input, prefix)
	if err != nil {
		errors = append(errors, fmt.Errorf("could not extract logs config: %v", err))
	}
	configs = append(configs, logsConfigs...)

	return configs, errors
}

// extractCheckTemplatesFromMap returns all the check configurations from a given map.
func extractCheckTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, error) {
	value, found := input[prefix+checkNamePath]
	if !found {
		return []integration.Config{}, nil
	}
	checkNames, err := parseCheckNames(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", checkNamePath, err)
	}

	value, found = input[prefix+initConfigPath]
	if !found {
		return []integration.Config{}, errors.New("missing init_configs key")
	}
	initConfigs, err := parseJSONValue(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", initConfigPath, err)
	}

	value, found = input[prefix+instancePath]
	if !found {
		return []integration.Config{}, errors.New("missing instances key")
	}
	instances, err := parseJSONValue(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", instancePath, err)
	}

	return buildTemplates(key, checkNames, initConfigs, instances), nil
}

// extractLogsTemplatesFromMap returns the logs configuration from a given map,
// if none are found return an empty list.
func extractLogsTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, error) {
	value, found := input[prefix+logsConfigPath]
	if !found {
		return []integration.Config{}, nil
	}
	var data interface{}
	err := json.Unmarshal([]byte(value), &data)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", logsConfigPath, err)
	}
	switch data.(type) {
	case []interface{}:
		logsConfig, _ := json.Marshal(data)
		return []integration.Config{{LogsConfig: logsConfig, ADIdentifiers: []string{key}}}, nil
	default:
		return []integration.Config{}, fmt.Errorf("invalid format, expected an array, got: '%v'", data)
	}
}

// GetPollInterval computes the poll interval from the config
func GetPollInterval(cp config.ConfigurationProviders) time.Duration {
	if cp.PollInterval != "" {
		customInterval, err := time.ParseDuration(cp.PollInterval)
		if err == nil {
			return customInterval
		}
	}
	return config.Datadog.GetDuration("ad_config_poll_interval") * time.Second
}
