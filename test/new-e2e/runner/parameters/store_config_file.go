// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"os"

	"encoding/json"

	"gopkg.in/yaml.v2"
)

// Configuration file example
// configParams:
//   aws:
//     keyPairName: "totoro"
//     publicKeyPath: "/home/totoro/.ssh/id_rsa.pub"
//   agent:
//     apiKey: "00000000000000000000000000000000"=
// stackParams:
//   ddinfra:
//     aws/someVariable: "ponyo"

type Config struct {
	ConfigParams ConfigParams                 `yaml:"configParams"`
	StackParams  map[string]map[string]string `yaml:"stackParams"`
}

type ConfigParams struct {
	AWS   AWS   `yaml:"aws"`
	Agent Agent `yaml:"agent"`
}
type AWS struct {
	KeyPairName   string `yaml:"keyPairName"`
	PublicKeyPath string `yaml:"publicKeyPath"`
}

type Agent struct {
	APIKey string `yaml:"apiKey"`
	APPKey string `yaml:"appKey"`
}

var _ valueStore = &configFileValueStore{}

type configFileValueStore struct {
	config          Config
	stackParamsJson string
}

func NewConfigFileValueStore(path string) (configFileValueStore, error) {
	store := configFileValueStore{}
	content, err := os.ReadFile(path)
	if err != nil {
		return store, err
	}

	err = store.parseConfigFileContent(content)

	return store, err
}

func (s *configFileValueStore) parseConfigFileContent(content []byte) error {
	err := yaml.Unmarshal(content, &s.config)
	if err != nil {
		return err
	}
	// parse StackParams to json string
	stackParams := map[string]string{}
	for namespace, submap := range s.config.StackParams {
		for key, value := range submap {
			stackParams[namespace+":"+key] = value
		}
	}
	b, err := json.Marshal(stackParams)
	if err != nil {
		return err
	}
	s.stackParamsJson = string(b)
	return err
}

// Get returns parameter value.
func (s configFileValueStore) get(key StoreKey) (string, error) {
	var value string

	switch key {
	case APIKey:
		value = s.config.ConfigParams.Agent.APIKey
	case APPKey:
		value = s.config.ConfigParams.Agent.APPKey
	case KeyPairName:
		value = s.config.ConfigParams.AWS.KeyPairName
	case PublicKeyPath:
		value = s.config.ConfigParams.AWS.PublicKeyPath
	case StackParameters:
		value = s.stackParamsJson
	}

	if value == "" {
		return value, ParameterNotFoundError{key: key}
	}

	return value, nil
}
