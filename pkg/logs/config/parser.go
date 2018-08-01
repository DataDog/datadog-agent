// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	json "github.com/json-iterator/go"
)

// Parse returns a new logs-configuration parsing the jsonString,
// if the parsing failed or the configuration is invalid, returns an error.
func Parse(jsonString string) ([]*LogsConfig, error) {
	var configs []LogsConfig
	var err error
	err = json.Unmarshal([]byte(jsonString), &configs)
	if err != nil {
		return nil, fmt.Errorf("could not parse logs config, invalid format: %v", jsonString)
	}
	var validConfigs []*LogsConfig
	for _, config := range configs {
		err = validateProcessingRules(config.ProcessingRules)
		if err != nil {
			log.Errorf("Invalid processing rules: %v", err)
			continue
		}
		err = compileProcessingRules(config.ProcessingRules)
		if err != nil {
			log.Errorf("Could not compile processing rules: %v", err)
		}
		validConfigs = append(validConfigs, &config)
	}
	return validConfigs, nil
}
