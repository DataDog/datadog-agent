// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"encoding/json"
	"errors"
	"maps"

	commonconfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	infraaws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	infraazure "github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	infragcp "github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	infralocal "github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

const (
	// AgentAPIKey pulumi config parameter name
	AgentAPIKey = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentAPIKeyParamName
	// AgentAPPKey pulumi config parameter name
	AgentAPPKey = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentAPPKeyParamName
	// AgentPipelineID pulumi config parameter name
	AgentPipelineID = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentPipelineID
	// AgentMajorVersion pulumi config parameter name
	AgentMajorVersion = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentMajorVersion
	// AgentCommitSHA pulumi config parameter name
	AgentCommitSHA = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentCommitSHA
	// AgentFIPS pulumi config parameter name
	AgentFIPS = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDAgentFIPS

	// InfraEnvironmentVariables pulumi config parameter name
	InfraEnvironmentVariables = commonconfig.DDInfraConfigNamespace + ":" + commonconfig.DDInfraEnvironment

	// InfraExtraResourcesTags pulumi config parameter name
	InfraExtraResourcesTags = commonconfig.DDInfraConfigNamespace + ":" + commonconfig.DDInfraExtraResourcesTags

	//InfraInitOnly pulumi config parameter name
	InfraInitOnly = commonconfig.DDInfraConfigNamespace + ":" + commonconfig.DDInfraInitOnly

	// ImagePullRegistry pulumi config parameter name
	ImagePullRegistry = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDImagePullRegistryParamName
	// ImagePullUsername pulumi config parameter name
	ImagePullUsername = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDImagePullUsernameParamName
	// ImagePullPassword pulumi config parameter name
	ImagePullPassword = commonconfig.DDAgentConfigNamespace + ":" + commonconfig.DDImagePullPasswordParamName

	// AWSKeyPairName pulumi config parameter name
	AWSKeyPairName = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDInfraDefaultKeyPairParamName
	// AWSPublicKeyPath pulumi config parameter name
	AWSPublicKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDinfraDefaultPublicKeyPath
	// AWSPrivateKeyPath pulumi config parameter name
	AWSPrivateKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDInfraDefaultPrivateKeyPath
	// AWSPrivateKeyPassword pulumi config parameter name
	AWSPrivateKeyPassword = commonconfig.DDInfraConfigNamespace + ":" + infraaws.DDInfraDefaultPrivateKeyPassword

	// AzurePublicKeyPath pulumi config paramater name
	AzurePublicKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infraazure.DDInfraDefaultPublicKeyPath
	// AzurePrivateKeyPath pulumi config paramater name
	AzurePrivateKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infraazure.DDInfraDefaultPrivateKeyPath
	// AzurePrivateKeyPassword pulumi config paramater name
	AzurePrivateKeyPassword = commonconfig.DDInfraConfigNamespace + ":" + infraazure.DDInfraDefaultPrivateKeyPassword

	// GCPPublicKeyPath pulumi config paramater name
	GCPPublicKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infragcp.DDInfraDefaultPublicKeyPath
	// GCPPrivateKeyPath pulumi config paramater name
	GCPPrivateKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infragcp.DDInfraDefaultPrivateKeyPath
	// GCPPrivateKeyPassword pulumi config paramater name
	GCPPrivateKeyPassword = commonconfig.DDInfraConfigNamespace + ":" + infragcp.DDInfraDefaultPrivateKeyPassword

	// LocalPublicKeyPath pulumi config paramater name
	LocalPublicKeyPath = commonconfig.DDInfraConfigNamespace + ":" + infralocal.DDInfraDefaultPublicKeyPath
)

// ConfigMap type alias to auto.ConfigMap
type ConfigMap auto.ConfigMap

// Set a value by key in a config map
func (cm ConfigMap) Set(key, val string, secret bool) {
	cm[key] = auto.ConfigValue{
		Value:  val,
		Secret: secret,
	}
}

// Merge in ConfigMap into current config map
func (cm ConfigMap) Merge(in ConfigMap) {
	maps.Copy(cm, in)
}

// ToPulumi casts current config map to a Pulumi auto.ConfigMap
func (cm ConfigMap) ToPulumi() auto.ConfigMap {
	return (auto.ConfigMap)(cm)
}

// SetConfigMapFromSecret set config map from a secret store
func SetConfigMapFromSecret(secretStore parameters.Store, cm ConfigMap, paramName parameters.StoreKey, configMapKey string) error {
	return setConfigMapFromParameter(secretStore, cm, paramName, configMapKey, true)
}

// SetConfigMapFromParameter set config map from a parameter store
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

// BuildStackParameters creates a config map from a profile, a scenario config map
// and env/cli configuration parameters
func BuildStackParameters(profile Profile, scenarioConfig ConfigMap) (ConfigMap, error) {
	var err error
	// Priority order: profile configs < scenarioConfig < Env/CLI config
	cm := ConfigMap{}

	// Parameters from profile
	cm.Set(InfraEnvironmentVariables, profile.EnvironmentNames(), false)
	params := map[parameters.StoreKey][]string{
		parameters.KeyPairName:         {AWSKeyPairName},
		parameters.AWSPublicKeyPath:    {AWSPublicKeyPath},
		parameters.AzurePublicKeyPath:  {AzurePublicKeyPath},
		parameters.GCPPublicKeyPath:    {GCPPublicKeyPath},
		parameters.AWSPrivateKeyPath:   {AWSPrivateKeyPath},
		parameters.AzurePrivateKeyPath: {AzurePrivateKeyPath},
		parameters.GCPPrivateKeyPath:   {GCPPrivateKeyPath},
		parameters.ImagePullRegistry:   {ImagePullRegistry},
		parameters.ImagePullUsername:   {ImagePullUsername},
		parameters.ImagePullPassword:   {ImagePullPassword},
		parameters.LocalPublicKeyPath:  {LocalPublicKeyPath},
		parameters.ExtraResourcesTags:  {InfraExtraResourcesTags},
		parameters.PipelineID:          {AgentPipelineID},
		parameters.FIPS:                {AgentFIPS},
		parameters.MajorVersion:        {AgentMajorVersion},
		parameters.CommitSHA:           {AgentCommitSHA},
		parameters.InitOnly:            {InfraInitOnly},
	}

	for storeKey, configMapKeys := range params {
		for _, configMapKey := range configMapKeys {

			err = SetConfigMapFromParameter(profile.ParamStore(), cm, storeKey, configMapKey)
			if err != nil {
				return nil, err
			}
		}
	}

	// Secret parameters from profile store
	secretParams := map[parameters.StoreKey][]string{
		parameters.APIKey:                  {AgentAPIKey},
		parameters.APPKey:                  {AgentAPPKey},
		parameters.AWSPrivateKeyPassword:   {AWSPrivateKeyPassword},
		parameters.AzurePrivateKeyPassword: {AzurePrivateKeyPassword},
		parameters.GCPPrivateKeyPassword:   {GCPPrivateKeyPassword},
	}

	for storeKey, configMapKeys := range secretParams {
		for _, configMapKey := range configMapKeys {
			err = SetConfigMapFromSecret(profile.SecretStore(), cm, storeKey, configMapKey)
			if err != nil {
				return nil, err
			}
		}
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
