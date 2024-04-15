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
//     privateKeyPath: "/home/totoro/.ssh/id_rsa"
//     privateKeyPassword: "princess_mononoke"
//   agent:
//     apiKey: "00000000000000000000000000000000"=
// stackParams:
//   ddinfra:
//     aws/someVariable: "ponyo"

// Config instance contains ConfigParams and StackParams
type Config struct {
	ConfigParams ConfigParams                 `yaml:"configParams"`
	StackParams  map[string]map[string]string `yaml:"stackParams"`
}

// ConfigParams instance contains config relayed parameters
type ConfigParams struct {
	AWS       AWS    `yaml:"aws"`
	Agent     Agent  `yaml:"agent"`
	OutputDir string `yaml:"outputDir"`
	Pulumi    Pulumi `yaml:"pulumi"`
	DevMode   string `yaml:"devMode"`
}

// AWS instance contains AWS related parameters
type AWS struct {
	Account            string `yaml:"account"`
	KeyPairName        string `yaml:"keyPairName"`
	PublicKeyPath      string `yaml:"publicKeyPath"`
	PrivateKeyPath     string `yaml:"privateKeyPath"`
	PrivateKeyPassword string `yaml:"privateKeyPassword"`
	TeamTag            string `yaml:"teamTag"`
}

// Agent instance contains agent related parameters
type Agent struct {
	APIKey              string `yaml:"apiKey"`
	APPKey              string `yaml:"appKey"`
	VerifyCodeSignature string `yaml:"verifyCodeSignature"`
}

// Pulumi instance contains pulumi related parameters
type Pulumi struct {
	// Sets the log level for Pulumi operations
	// Be careful setting this value, as it can expose sensitive information in the logs.
	// https://www.pulumi.com/docs/support/troubleshooting/#verbose-logging
	LogLevel string `yaml:"logLevel"`
	// By default pulumi logs to /tmp, and creates symlinks to the most recent log, e.g. /tmp/pulumi.INFO
	// Set this option to true to log to stderr instead.
	// https://www.pulumi.com/docs/support/troubleshooting/#verbose-logging
	LogToStdErr string `yaml:"logToStdErr"`
	// To reduce logs noise in the CI, by default we display only the Pulumi error progress steam.
	// Set this option to true to disaply all the progress streams.
	VerboseProgressStreams string `yaml:"verboseProgressStreams"`
}

var _ valueStore = &configFileValueStore{}

type configFileValueStore struct {
	config          Config
	stackParamsJSON string
}

// NewConfigFileStore creates a store from configFileValueStore from a path
func NewConfigFileStore(path string) (Store, error) {
	valueStore := configFileValueStore{}
	content, err := os.ReadFile(path)
	if err != nil {
		return newStore(valueStore), err
	}

	err = valueStore.parseConfigFileContent(content)

	return newStore(valueStore), err
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
	s.stackParamsJSON = string(b)
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
	case PrivateKeyPath:
		value = s.config.ConfigParams.AWS.PrivateKeyPath
	case PrivateKeyPassword:
		value = s.config.ConfigParams.AWS.PrivateKeyPassword
	case StackParameters:
		value = s.stackParamsJSON
	case Environments:
		if s.config.ConfigParams.AWS.Account != "" {
			value = "aws/" + s.config.ConfigParams.AWS.Account
		}
	case ExtraResourcesTags:
		if s.config.ConfigParams.AWS.TeamTag != "" {
			value = "team:" + s.config.ConfigParams.AWS.TeamTag
		}
	case VerifyCodeSignature:
		value = s.config.ConfigParams.Agent.VerifyCodeSignature
	case OutputDir:
		value = s.config.ConfigParams.OutputDir
	case PulumiLogLevel:
		value = s.config.ConfigParams.Pulumi.LogLevel
	case PulumiLogToStdErr:
		value = s.config.ConfigParams.Pulumi.LogToStdErr
	case PulumiVerboseProgressStreams:
		value = s.config.ConfigParams.Pulumi.VerboseProgressStreams
	case DevMode:
		value = s.config.ConfigParams.DevMode
	}

	if value == "" {
		return value, ParameterNotFoundError{key: key}
	}

	return value, nil
}
