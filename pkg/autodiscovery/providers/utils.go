// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"errors"
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	json "github.com/json-iterator/go"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
)

func init() {
	// Where to look for check templates if no custom path is defined
	config.Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	// Defaut Timeout in second when talking to storage for configuration (etcd, zookeeper, ...)
	config.Datadog.SetDefault("autoconf_template_url_timeout", 5)
}

// parseJSONValue returns a slice of ConfigData parsed from the JSON
// contained in the `value` parameter
func parseJSONValue(value string) ([]integration.Data, error) {
	if value == "" {
		return nil, fmt.Errorf("Value is empty")
	}

	var rawRes []interface{}
	var result []integration.Data

	err := json.Unmarshal([]byte(value), &rawRes)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal JSON: %s", err)
	}

	for _, r := range rawRes {
		switch r.(type) {
		case map[string]interface{}:
			init, _ := json.Marshal(r)
			result = append(result, init)
		default:
			return nil, fmt.Errorf("found non JSON object type, value is: '%v'", r)
		}

	}
	return result, nil
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

func buildTemplates(key string, checkNames []string, initConfigs, instances []integration.Data) []integration.Config {
	templates := make([]integration.Config, 0)

	// sanity check
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length, not using them.")
		return templates
	}

	for idx := range checkNames {
		instance := integration.Data(instances[idx])

		templates = append(templates, integration.Config{
			Name:          checkNames[idx],
			InitConfig:    integration.Data(initConfigs[idx]),
			Instances:     []integration.Data{instance},
			ADIdentifiers: []string{key},
		})
	}
	return templates
}

// extractTemplatesFromMap looks for autodiscovery configurations in a given map
// (either docker labels or kubernetes annotations) and returns them if found.
func extractTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, error) {
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
