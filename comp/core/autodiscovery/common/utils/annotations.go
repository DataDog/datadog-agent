// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	instancePath            = "instances"
	checkNamePath           = "check_names"
	initConfigPath          = "init_configs"
	ignoreAutodiscoveryTags = "ignore_autodiscovery_tags"
	logsConfigPath          = "logs"
	checksPath              = "checks"
	checkIDPath             = "check.id"
	checkTagCardinality     = "check_tag_cardinality"
)

// ExtractTemplatesFromMap looks for autodiscovery configurations in a given
// map and returns them if found.
func ExtractTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, []error) {
	var configs []integration.Config
	var errors []error

	checksConfigs, err := extractCheckTemplatesFromMap(key, input, prefix)
	if err != nil {
		errors = append(errors, fmt.Errorf("could not extract checks config: %v", err))
	}
	configs = append(configs, checksConfigs...)

	logsConfigs, err := extractLogsTemplatesFromMap(configs, key, input, prefix)
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
	checkNames, err := ParseCheckNames(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", checkNamePath, err)
	}

	value, found = input[prefix+initConfigPath]
	if !found {
		return []integration.Config{}, errors.New("missing init_configs key")
	}
	initConfigs, err := ParseJSONValue(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", initConfigPath, err)
	}

	value, found = input[prefix+instancePath]
	if !found {
		return []integration.Config{}, errors.New("missing instances key")
	}
	instances, err := ParseJSONValue(value)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", instancePath, err)
	}
	// ParseBool returns `true` only on success cases
	ignoreAdTags, _ := strconv.ParseBool(input[prefix+ignoreAutodiscoveryTags])

	cardinality := input[prefix+checkTagCardinality]

	return BuildTemplates(key, checkNames, initConfigs, instances, ignoreAdTags, cardinality), nil
}

// extractLogsTemplatesFromMap returns the logs configuration from a given map,
// if none are found return an empty list.
func extractLogsTemplatesFromMap(configs []integration.Config, key string, input map[string]string, prefix string) ([]integration.Config, error) {
	value, found := input[prefix+logsConfigPath]
	if !found {
		return []integration.Config{}, nil
	}
	logCheckName := ""
	if len(configs) >= 1 {
		// Consider the first check name as the log check name, even if it's empty
		// It's possible to have different names in different configs, and it would mean that one attached multiple integrations
		// to a single container (e.g. redis + nginx). We expect we won't encounter this most of the time,
		// but if it happens it means we're tagging the wrong integration name.
		logCheckName = configs[0].Name
	}

	var data interface{}
	err := json.Unmarshal([]byte(value), &data)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", logsConfigPath, err)
	}
	switch data.(type) {
	case []interface{}:
		logsConfig, _ := json.Marshal(data)
		return []integration.Config{{Name: logCheckName, LogsConfig: logsConfig, ADIdentifiers: []string{key}}}, nil
	default:
		return []integration.Config{}, fmt.Errorf("invalid format, expected an array, got: '%v'", data)
	}
}

// ParseCheckNames returns a slice of check names parsed from a JSON array
func ParseCheckNames(names string) (res []string, err error) {
	if names == "" {
		return nil, fmt.Errorf("check_names is empty")
	}

	if err = json.Unmarshal([]byte(names), &res); err != nil {
		return nil, err
	}

	return res, nil
}

// ParseJSONValue returns a slice of slice of ConfigData parsed from the JSON
// contained in the `value` parameter
func ParseJSONValue(value string) ([][]integration.Data, error) {
	if value == "" {
		return nil, fmt.Errorf("value is empty")
	}

	var rawRes []interface{}
	var result [][]integration.Data

	err := json.Unmarshal([]byte(value), &rawRes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	for _, r := range rawRes {
		switch objs := r.(type) {
		case []interface{}:
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

// BuildTemplates returns check configurations configured according to the
// passed in AD identifier, check names, init, instance configs and their
// `ignoreAutoDiscoveryTags`, `CheckTagCardinality` fields.
func BuildTemplates(adID string, checkNames []string, initConfigs, instances [][]integration.Data, ignoreAutodiscoveryTags bool, checkCard string) []integration.Config {
	templates := make([]integration.Config, 0)

	// sanity checks
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Errorf("Template entries don't all have the same length. "+
			"checkNames: %d, initConfigs: %d, instances: %d. Not using them.",
			len(checkNames), len(initConfigs), len(instances))
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
				Name:                    checkNames[idx],
				InitConfig:              initConfigs[idx][0],
				Instances:               []integration.Data{instance},
				ADIdentifiers:           []string{adID},
				IgnoreAutodiscoveryTags: ignoreAutodiscoveryTags,
				CheckTagCardinality:     checkCard,
			})
		}
	}
	return templates
}

func parseJSONObjToData(r interface{}) (integration.Data, error) {
	switch r.(type) {
	case map[string]interface{}:
		return json.Marshal(r)
	default:
		return nil, fmt.Errorf("found non JSON object type, value is: '%v'", r)
	}
}

func extractCheckNamesFromMap(annotations map[string]string, prefix string, legacyPrefix string) ([]string, error) {
	// AD annotations v2: "ad.datadoghq.com/redis.checks"
	if checksJSON, found := annotations[prefix+checksPath]; found {
		// adIdentifier is an empty string since it's needed by
		// parseChecksJSON but not used by this func
		var adIdentifier string
		checks, err := parseChecksJSON(adIdentifier, checksJSON)
		if err != nil {
			return nil, err
		}

		checkNames := make([]string, 0, len(checks))
		for _, config := range checks {
			checkNames = append(checkNames, config.Name)
		}

		return checkNames, nil
	}

	// AD annotations v1: "ad.datadoghq.com/redis.check_names"
	if checkNamesJSON, found := annotations[prefix+checkNamePath]; found {
		checkNames, err := ParseCheckNames(checkNamesJSON)
		if err != nil {
			return nil, fmt.Errorf("cannot parse check names: %w", err)
		}

		return checkNames, nil
	}

	if legacyPrefix != "" {
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
		if checkNamesJSON, found := annotations[legacyPrefix+checkNamePath]; found {
			checkNames, err := ParseCheckNames(checkNamesJSON)
			if err != nil {
				return nil, fmt.Errorf("cannot parse check names: %w", err)
			}

			return checkNames, nil
		}
	}

	return nil, nil
}

func extractTemplatesFromMapWithV2(entityName string, annotations map[string]string, prefix string, legacyPrefix string) ([]integration.Config, []error) {
	var (
		configs      []integration.Config
		errors       []error
		actualPrefix string
	)

	var prefixCandidates = []string{prefix}
	if legacyPrefix != "" {
		prefixCandidates = append(prefixCandidates, legacyPrefix)
	}

	if checksJSON, found := annotations[prefix+checksPath]; found {
		// AD annotations v2: "ad.datadoghq.com/redis.checks"
		actualPrefix = prefix
		c, err := parseChecksJSON(entityName, checksJSON)
		if err != nil {
			errors = append(errors, err)
		} else {
			configs = append(configs, c...)
		}
	} else {
		// AD annotations v1: "ad.datadoghq.com/redis.check_names"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
		actualPrefix = findPrefix(annotations, prefixCandidates, checkNamePath)

		if actualPrefix != "" {
			c, err := extractCheckTemplatesFromMap(entityName, annotations, actualPrefix)
			if err != nil {
				errors = append(errors, fmt.Errorf("could not extract checks config: %v", err))
			} else {
				configs = append(configs, c...)
			}
		}
	}

	// prefix might not have been detected if there are no check
	// annotations, so we try to find a prefix for log configs
	if actualPrefix == "" {
		// AD annotations v1: "ad.datadoghq.com/redis.logs"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.logs"
		actualPrefix = findPrefix(annotations, prefixCandidates, logsConfigPath)
	}

	if actualPrefix != "" {
		c, err := extractLogsTemplatesFromMap(configs, entityName, annotations, actualPrefix)

		if err != nil {
			errors = append(errors, fmt.Errorf("could not extract logs config: %v", err))
		} else {
			configs = append(configs, c...)
		}
	}

	return configs, errors
}

func findPrefix(annotations map[string]string, prefixes []string, suffix string) string {
	for _, prefix := range prefixes {
		key := prefix + suffix
		if _, ok := annotations[key]; ok {
			return prefix
		}
	}

	return ""
}
