// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/viper"
)

// ParseJSON parses the data formatted in JSON
// returns an error if the parsing failed.
func ParseJSON(data []byte) ([]*LogsConfig, error) {
	var configs []*LogsConfig
	err := json.Unmarshal(data, &configs)
	if err != nil {
		return nil, fmt.Errorf("could not parse JSON logs config: %v", err)
	}
	return configs, nil
}

const yaml = "yaml"
const logsPath = "logs"

// ParseYAML parses the data formatted in YAML,
// returns an error if the parsing failed.
func ParseYAML(data []byte) ([]*LogsConfig, error) {
	var configs []*LogsConfig
	var err error
	v := viper.New()
	v.SetConfigType(yaml)
	err = v.ReadConfig(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("could not decode YAML logs config: %v", err)
	}
	err = v.UnmarshalKey(logsPath, &configs)
	if err != nil {
		return nil, fmt.Errorf("could not parse YAML logs config: %v", err)
	}
	return configs, nil
}
