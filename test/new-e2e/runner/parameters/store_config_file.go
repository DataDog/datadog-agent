// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	StackParams StackParams `yaml:"stackParams"`
}

type StackParams struct {
	DDInfra DDInfra `yaml:"ddinfra"`
}

type DDInfra struct {
	AWS AWS `yaml:"aws"`
}

type AWS struct {
	DefaultKeyPairName string `yaml:"defaultKeyPairName"`
}

type configFileValueStore struct {
	values map[string]string
}

func NewConfigFileValueStore(path string) (configFileValueStore, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return configFileValueStore{}, err
	}

	config := Config{}
	err = yaml.Unmarshal(content, &config)
	if err != nil {
		return configFileValueStore{}, err
	}

	valueStore := configFileValueStore{values: make(map[string]string)}

	defaultKeyPairName := config.StackParams.DDInfra.AWS.DefaultKeyPairName
	if defaultKeyPairName != "" {
		valueStore.values[GetDefaultKeyPairParamName()] = defaultKeyPairName
	}
	return valueStore, nil
}

// Get returns parameter value.
func (s configFileValueStore) get(key string) (string, error) {
	value, ok := s.values[key]
	if ok {
		return value, nil
	}
	return "", ParameterNotFoundError{key: key}
}
