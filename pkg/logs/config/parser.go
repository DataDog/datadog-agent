// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
)

// Parse returns a new logs-configuration parsing the jsonString,
// if the parsing failed or the configuration is invalid, returns an error.
func Parse(jsonString string) (*LogsConfig, error) {
	var configs []LogsConfig
	var err error
	err = json.Unmarshal([]byte(jsonString), &configs)
	if err != nil || len(configs) == 0 {
		return nil, fmt.Errorf("could not parse logs config, invalid format: %v", jsonString)
	}
	config := configs[0]
	err = ValidateProcessingRules(config.ProcessingRules)
	if err != nil {
		return nil, fmt.Errorf("invalid processing rules: %v", err)
	}
	err = CompileProcessingRules(config.ProcessingRules)
	if err != nil {
		return nil, fmt.Errorf("could not compile processing rules: %v", err)
	}
	return &config, nil
}
