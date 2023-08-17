// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	commonconfig "github.com/DataDog/test-infra-definitions/common/config"
	infraaws "github.com/DataDog/test-infra-definitions/resources/aws"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

const (
	AgentAPIKey     = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentAPIKeyParamName
	AgentAPPKey     = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentAPPKeyParamName
	AgentPipelineID = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentPipelineID

	InfraEnvironmentVariables = commonconfig.DDInfraConfigNamespace + ":" + commonconfig.DDInfraEnvironment
	InfraExtraResourcesTags   = commonconfig.DDInfraConfigNamespace + ":" + commonconfig.DDInfraExtraResourcesTags

	AWSKeyPairName    = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDInfraDefaultKeyPairParamName
	AWSPublicKeyPath  = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDinfraDefaultPublicKeyPath
	AWSPrivateKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDInfraDefaultPrivateKeyPath
)

type ConfigMap auto.ConfigMap

func (cm ConfigMap) Set(key, val string, secret bool) {
	cm[key] = auto.ConfigValue{
		Value:  val,
		Secret: secret,
	}
}

func (cm ConfigMap) Merge(in ConfigMap) {
	for key, val := range in {
		cm[key] = val
	}
}

func (cm ConfigMap) ToPulumi() auto.ConfigMap {
	return (auto.ConfigMap)(cm)
}

func SetConfigMapFromSecret(secretStore parameters.Store, cm ConfigMap, paramName parameters.StoreKey, configMapKey string) error {
	return setConfigMapFromParameter(secretStore, cm, paramName, configMapKey, true)
}

func SetConfigMapFromParameter(store parameters.Store, cm ConfigMap, paramName parameters.StoreKey, configMapKey string) error {
	return setConfigMapFromParameter(store, cm, paramName, configMapKey, false)
}

func setConfigMapFromParameter(store parameters.Store, cm ConfigMap, paramName parameters.StoreKey, configMapKey string, secret bool) error {
	val, err := store.Get(paramName)
	if err != nil {
		if errors.As(err, &parameters.ParameterNotFoundError{}) {
			return nil
		}
		return err
	}

	cm[configMapKey] = auto.ConfigValue{
		Value:  val,
		Secret: secret,
	}
	return nil
}

func BuildStackParameters(profile Profile, scenarioConfig ConfigMap) (ConfigMap, error) {
	// Priority order: profile configs < scenarioConfig < Env/CLI config
	cm := ConfigMap{}

	// Parameters from profile
	cm.Set("ddinfra:env", profile.EnvironmentNames(), false)
	err := SetConfigMapFromParameter(profile.ParamStore(), cm, parameters.KeyPairName, AWSKeyPairName)
	if err != nil {
		return nil, err
	}
	err = SetConfigMapFromParameter(profile.ParamStore(), cm, parameters.PublicKeyPath, AWSPublicKeyPath)
	if err != nil {
		return nil, err
	}
	err = SetConfigMapFromParameter(profile.ParamStore(), cm, parameters.PrivateKeyPath, AWSPrivateKeyPath)
	if err != nil {
		return nil, err
	}
	err = SetConfigMapFromParameter(profile.ParamStore(), cm, parameters.ExtraResourcesTags, InfraExtraResourcesTags)
	if err != nil {
		return nil, err
	}
	err = SetConfigMapFromParameter(profile.ParamStore(), cm, parameters.PipelineID, AgentPipelineID)
	if err != nil {
		return nil, err
	}
	// Secret parameters from profile store
	err = SetConfigMapFromSecret(profile.SecretStore(), cm, parameters.APIKey, AgentAPIKey)
	if err != nil {
		return nil, err
	}
	err = SetConfigMapFromSecret(profile.SecretStore(), cm, parameters.APPKey, AgentAPPKey)
	if err != nil {
		return nil, err
	}
	// Merge with scenario variables
	cm.Merge(scenarioConfig)

	// Read Env/CLI config
	stackParamsJSON, err := profile.ParamStore().GetWithDefault(parameters.StackParameters, "")
	if err != nil {
		return nil, err
	}
	if stackParamsJSON != "" {
		var stackParams map[string]string
		if err := json.Unmarshal([]byte(stackParamsJSON), &stackParams); err != nil {
			return nil, err
		}

		for key, val := range stackParams {
			cm.Set(key, val, false)
		}
	}

	return cm, nil
}
