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

// SetConfigMapFromStore sets a config map value from the given store, marking it as secret or not.
func SetConfigMapFromStore(store parameters.Store, cm ConfigMap, paramName parameters.StoreKey, configMapKey string, secret bool) error {
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
	type paramEntry struct {
		store         parameters.Store
		configMapKeys []string
		secret        bool
	}

	allParams := map[parameters.StoreKey]paramEntry{
		parameters.KeyPairName:             {profile.ParamStore(), []string{AWSKeyPairName}, false},
		parameters.AWSPublicKeyPath:        {profile.ParamStore(), []string{AWSPublicKeyPath}, false},
		parameters.AzurePublicKeyPath:      {profile.ParamStore(), []string{AzurePublicKeyPath}, false},
		parameters.GCPPublicKeyPath:        {profile.ParamStore(), []string{GCPPublicKeyPath}, false},
		parameters.AWSPrivateKeyPath:       {profile.ParamStore(), []string{AWSPrivateKeyPath}, false},
		parameters.AzurePrivateKeyPath:     {profile.ParamStore(), []string{AzurePrivateKeyPath}, false},
		parameters.GCPPrivateKeyPath:       {profile.ParamStore(), []string{GCPPrivateKeyPath}, false},
		parameters.ImagePullRegistry:       {profile.ParamStore(), []string{ImagePullRegistry}, false},
		parameters.ImagePullUsername:       {profile.ParamStore(), []string{ImagePullUsername}, false},
		parameters.ImagePullPassword:       {profile.ParamStore(), []string{ImagePullPassword}, true},
		parameters.LocalPublicKeyPath:      {profile.ParamStore(), []string{LocalPublicKeyPath}, false},
		parameters.ExtraResourcesTags:      {profile.ParamStore(), []string{InfraExtraResourcesTags}, false},
		parameters.PipelineID:              {profile.ParamStore(), []string{AgentPipelineID}, false},
		parameters.FIPS:                    {profile.ParamStore(), []string{AgentFIPS}, false},
		parameters.MajorVersion:            {profile.ParamStore(), []string{AgentMajorVersion}, false},
		parameters.CommitSHA:               {profile.ParamStore(), []string{AgentCommitSHA}, false},
		parameters.InitOnly:                {profile.ParamStore(), []string{InfraInitOnly}, false},
		parameters.APIKey:                  {profile.SecretStore(), []string{AgentAPIKey}, true},
		parameters.APPKey:                  {profile.SecretStore(), []string{AgentAPPKey}, true},
		parameters.AWSPrivateKeyPassword:   {profile.SecretStore(), []string{AWSPrivateKeyPassword}, true},
		parameters.AzurePrivateKeyPassword: {profile.SecretStore(), []string{AzurePrivateKeyPassword}, true},
		parameters.GCPPrivateKeyPassword:   {profile.SecretStore(), []string{GCPPrivateKeyPassword}, true},
	}

	for storeKey, entry := range allParams {
		for _, configMapKey := range entry.configMapKeys {
			err = SetConfigMapFromStore(entry.store, cm, storeKey, configMapKey, entry.secret)
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
