// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"

	yaml "gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type yamlLogsConfigsWrapper struct {
	Logs []*LogsConfig
}

// ParseJSON parses the data formatted in JSON
// returns an error if the parsing failed.
func ParseJSON(data []byte) ([]*LogsConfig, error) {
	var configs []*LogsConfig
	log.Debugf("Parsing JSON logs config: %s", string(data))
	err := json.Unmarshal(data, &configs)
	if err != nil {
		return nil, fmt.Errorf("could not parse JSON logs config: %v", err)
	}
	for _, cfg := range configs {
		log.Debugf("Parsed JSON logs config: %#v", cfg)
	}
	return configs, nil
}

// ParseYAML parses the data formatted in YAML,
// returns an error if the parsing failed.
func ParseYAML(data []byte) ([]*LogsConfig, error) {
	var yamlConfigsWrapper yamlLogsConfigsWrapper
	err := yaml.Unmarshal(data, &yamlConfigsWrapper)
	if err != nil {
		return nil, fmt.Errorf("could not decode YAML logs config: %v", err)
	}
	for _, cfg := range yamlConfigsWrapper.Logs {
		log.Debugf("Parsed YAML logs config: %+v", cfg)
	}
	return yamlConfigsWrapper.Logs, nil
}

// ParseJSONOrYAML parses the data trying Json first, then with a fallback to YAML.
func ParseJSONOrYAML(data []byte) ([]*LogsConfig, error) {
	if configs, err := ParseJSON(data); err == nil {
		return configs, nil
	}
	configs, err := ParseYAML(data)
	if err == nil {
		return configs, nil
	}
	return nil, fmt.Errorf("could not parse logs config as JSON or YAML: %v", err)
}
