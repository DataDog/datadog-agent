// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// KubeAnnotationPrefix is the prefix used by AD in Kubernetes
	// annotations.
	KubeAnnotationPrefix = "ad.datadoghq.com/"

	instancePath   = "instances"
	checkNamePath  = "check_names"
	initConfigPath = "init_configs"
	logsConfigPath = "logs"
	checksPath     = "checks"
	checkIDPath    = "check.id"

	legacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"

	podAnnotationFormat       = KubeAnnotationPrefix + "%s."
	legacyPodAnnotationFormat = legacyPodAnnotationPrefix + "%s."

	checkIDAnnotationFormat = podAnnotationFormat + checkIDPath

	v1PodAnnotationCheckNamesFormat     = podAnnotationFormat + checkNamePath
	v2PodAnnotationChecksFormat         = podAnnotationFormat + checksPath
	legacyPodAnnotationCheckNamesFormat = legacyPodAnnotationFormat + checkNamePath
)

var adPrefixFormats = []string{podAnnotationFormat, legacyPodAnnotationFormat}

// GetCustomCheckID returns whether there is a custom check ID for a given
// container based on the pod annotations
func GetCustomCheckID(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(checkIDAnnotationFormat, containerName)]
	return id, found
}

// ExtractCheckNames returns check names from a map of pod annotations. In order of
// priority, it prefers annotations v2, v1, and legacy.
func ExtractCheckNames(annotations map[string]string, adIdentifier string) ([]string, error) {
	// AD annotations v2: "ad.datadoghq.com/redis.checks"
	if checksJSON, found := annotations[fmt.Sprintf(v2PodAnnotationChecksFormat, adIdentifier)]; found {
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
	if checkNamesJSON, found := annotations[fmt.Sprintf(v1PodAnnotationCheckNamesFormat, adIdentifier)]; found {
		checkNames, err := ParseCheckNames(checkNamesJSON)
		if err != nil {
			return nil, fmt.Errorf("cannot parse check names: %w", err)
		}

		return checkNames, nil
	}

	// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
	if checkNamesJSON, found := annotations[fmt.Sprintf(legacyPodAnnotationCheckNamesFormat, adIdentifier)]; found {
		checkNames, err := ParseCheckNames(checkNamesJSON)
		if err != nil {
			return nil, fmt.Errorf("cannot parse check names: %w", err)
		}

		return checkNames, nil
	}

	return nil, nil
}

// ExtractTemplatesFromAnnotations looks for autodiscovery configurations in
// a map of annotations and returns them if found. In order of priority, it
// prefers annotations v2, v1, and legacy.
func ExtractTemplatesFromAnnotations(entityName string, annotations map[string]string, adIdentifier string) ([]integration.Config, []error) {
	var (
		configs []integration.Config
		errors  []error
		prefix  string
	)

	if checksJSON, found := annotations[fmt.Sprintf(v2PodAnnotationChecksFormat, adIdentifier)]; found {
		// AD annotations v2: "ad.datadoghq.com/redis.checks"
		prefix = fmt.Sprintf(podAnnotationFormat, adIdentifier)
		c, err := parseChecksJSON(entityName, checksJSON)
		if err != nil {
			errors = append(errors, err)
		} else {
			configs = append(configs, c...)
		}
	} else {
		// AD annotations v1: "ad.datadoghq.com/redis.check_names"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
		prefix = findPrefix(annotations, adPrefixFormats, adIdentifier, checkNamePath)
		if prefix != "" {
			c, err := extractCheckTemplatesFromMap(entityName, annotations, prefix)
			if err != nil {
				errors = append(errors, fmt.Errorf("could not extract checks config: %v", err))
			} else {
				configs = append(configs, c...)
			}
		}
	}

	// prefix might not have been detected if there are no check
	// annotations, so we try to find a prefix for log configs
	if prefix == "" {
		// AD annotations v1: "ad.datadoghq.com/redis.logs"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.logs"
		prefix = findPrefix(annotations, adPrefixFormats, adIdentifier, logsConfigPath)
	}

	if prefix != "" {
		c, err := extractLogsTemplatesFromMap(entityName, annotations, prefix)
		if err != nil {
			errors = append(errors, fmt.Errorf("could not extract logs config: %v", err))
		} else {
			configs = append(configs, c...)
		}
	}

	return configs, errors
}

// parseChecksJSON parses an AD annotation v2
// (ad.datadoghq.com/redis.checks) JSON string into []integration.Config.
func parseChecksJSON(adIdentifier string, checksJSON string) ([]integration.Config, error) {
	var namedChecks map[string]struct {
		Name       string            `json:"name"`
		InitConfig *integration.Data `json:"init_config"`
		Instances  []interface{}     `json:"instances"`
	}

	err := json.Unmarshal([]byte(checksJSON), &namedChecks)
	if err != nil {
		return nil, fmt.Errorf("cannot parse check configuration: %w", err)
	}

	checks := make([]integration.Config, 0, len(namedChecks))
	for name, config := range namedChecks {
		if config.Name != "" {
			name = config.Name
		}

		var initConfig integration.Data
		if config.InitConfig != nil {
			initConfig = *config.InitConfig
		} else {
			initConfig = integration.Data("{}")
		}

		c := integration.Config{
			Name:          name,
			InitConfig:    initConfig,
			ADIdentifiers: []string{adIdentifier},
		}

		for _, i := range config.Instances {
			instance, err := parseJSONObjToData(i)
			if err != nil {
				return nil, err
			}

			c.Instances = append(c.Instances, instance)
		}

		checks = append(checks, c)
	}

	return checks, nil
}

func findPrefix(annotations map[string]string, prefixFmts []string, adIdentifier, suffix string) string {
	for _, prefixFmt := range prefixFmts {
		key := fmt.Sprintf(prefixFmt+suffix, adIdentifier)
		if _, ok := annotations[key]; ok {
			return fmt.Sprintf(prefixFmt, adIdentifier)
		}
	}

	return ""
}

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

	return BuildTemplates(key, checkNames, initConfigs, instances), nil
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

// BuildTemplates returns check configurations configured according to the
// passed in AD identifier, check names, init and instance configs
func BuildTemplates(adID string, checkNames []string, initConfigs, instances [][]integration.Data) []integration.Config {
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
				InitConfig:    initConfigs[idx][0],
				Instances:     []integration.Data{instance},
				ADIdentifiers: []string{adID},
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
