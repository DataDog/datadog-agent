// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
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

func SetConfigMapFromParameter(store parameters.Store, cm ConfigMap, paramName, configMapKey string) error {
	val, err := store.Get(paramName)
	if err != nil {
		return err
	}

	cm[configMapKey] = auto.ConfigValue{
		Value: val,
	}
	return nil
}

func SetConfigMapFromSecret(secretStore parameters.Store, cm ConfigMap, paramName, configMapKey string) error {
	val, err := secretStore.Get(paramName)
	if err != nil {
		return err
	}

	cm[configMapKey] = auto.ConfigValue{
		Value:  val,
		Secret: true,
	}
	return nil
}

func BuildStackParameters(profile Profile, scenarioConfig ConfigMap) (ConfigMap, error) {
	// Priority order: profile configs < scenarioConfig < Env/CLI config

	// Inject profile variables
	cm := ConfigMap{}
	cm.Set("ddinfra:env", profile.EnvironmentNames(), false)
	err := SetConfigMapFromSecret(profile.SecretStore(), cm, parameters.APIKey, "ddagent:apiKey")
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
