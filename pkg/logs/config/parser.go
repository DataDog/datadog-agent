// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	json "github.com/json-iterator/go"
)

func ParseJSON(data []byte) ([]*LogsConfig, error) {
	return parse(data, json.Unmarshal)
}

func ParseYaml(data []byte) ([]*LogsConfig, error) {
	return parse(data, yaml.Unmarshal)
}

func parse(data []byte, unmarshal func(data []byte, v interface{}) error) ([]*LogsConfig, error) {
	var configs []*LogsConfig
	var err error
	err = unmarshal(data, &configs)
	if err != nil {
		return nil, fmt.Errorf("could not parse logs config, invalid format: %v", err)
	}
	return configs, nil
}
