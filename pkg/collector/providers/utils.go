// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
)

// ADEntryIndex structure to store indeces to backend entries
type ADEntryIndex struct {
	NamesIdx     uint64
	InitIdx      uint64
	InstancesIdx uint64
}

func init() {
	// Where to look for check templates if no custom path is defined
	config.Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	// Defaut Timeout in second when talking to storage for configuration (etcd, zookeeper, ...)
	config.Datadog.SetDefault("autoconf_template_url_timeout", 5)
}

// parseJSONValue returns a slice of ConfigData parsed from the JSON
// contained in the `value` parameter
func parseJSONValue(value string) ([]check.ConfigData, error) {
	if value == "" {
		return nil, fmt.Errorf("Value is empty")
	}

	var rawRes []interface{}
	var result []check.ConfigData

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

func buildTemplates(key string, checkNames []string, initConfigs, instances []check.ConfigData) []check.Config {
	templates := make([]check.Config, 0)

	// sanity check
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length, not using them.")
		return templates
	}

	for idx := range checkNames {
		instance := check.ConfigData(instances[idx])

		templates = append(templates, check.Config{
			Name:          checkNames[idx],
			InitConfig:    check.ConfigData(initConfigs[idx]),
			Instances:     []check.ConfigData{instance},
			ADIdentifiers: []string{key},
		})
	}
	return templates
}
