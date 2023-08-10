// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

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

// Config exported type should have comment or be unexported
type Config struct {
	ConfigParams ConfigParams                 `yaml:"configParams"`
	StackParams  map[string]map[string]string `yaml:"stackParams"`
}

// ConfigParams exported type should have comment or be unexported
type ConfigParams struct {
	AWS   AWS   `yaml:"aws"`
	Agent Agent `yaml:"agent"`
}

// AWS exported type should have comment or be unexported
type AWS struct {
	Account       string `yaml:"account"`
	KeyPairName   string `yaml:"keyPairName"`
	PublicKeyPath string `yaml:"publicKeyPath"`
	TeamTag       string `yaml:"teamTag"`
}

// Agent exported type should have comment or be unexported
type Agent struct {
	APIKey string `yaml:"apiKey"`
	APPKey string `yaml:"appKey"`
}

var _ valueStore = &ConfigFileValueStore{}

// ConfigFileValueStore exported type should have comment or be unexported
type ConfigFileValueStore struct {
	config Config
	// struct field stackParamsJson should be stackParamsJSON
	stackParamsJson string
}

// NewConfigFileValueStore exported function should have comment or be unexported
func NewConfigFileValueStore(path string) (ConfigFileValueStore, error) {
	store := ConfigFileValueStore{}
	content, err := os.ReadFile(path)
	if err != nil {
		return store, err
	}

	err = store.parseConfigFileContent(content)

	return store, err
}

func (s *ConfigFileValueStore) parseConfigFileContent(content []byte) error {
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
func (s ConfigFileValueStore) get(key StoreKey) (string, error) {
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
	case Environments:
		if s.config.ConfigParams.AWS.Account != "" {
			value = "aws/" + s.config.ConfigParams.AWS.Account
		}
	case ExtraResourcesTags:
		if s.config.ConfigParams.AWS.TeamTag != "" {
			value = "team:" + s.config.ConfigParams.AWS.TeamTag
		}
	}

	if value == "" {
		return value, ParameterNotFoundError{key: key}
	}

	return value, nil
}
