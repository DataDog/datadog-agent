// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdkconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
)

const (
	multiValueSeparator = ","

	namerNamespace             = "common"
	DDInfraConfigNamespace     = "ddinfra"
	DDAgentConfigNamespace     = "ddagent"
	DDTestingWorkloadNamespace = "ddtestworkload"
	DDDogstatsdNamespace       = "dddogstatsd"
	DDUpdaterConfigNamespace   = "ddupdater"
	DDOperatorConfigNamespace  = "ddoperator"

	// Infra namespace
	DDInfraEnvironment                      = "env"
	DDInfraKubernetesVersion                = "kubernetesVersion"
	DDInfraKindVersion                      = "kindVersion"
	DDInfraKubeNodeURL                      = "kubeNodeUrl"
	DDInfraOSDescriptor                     = "osDescriptor" // osDescriptor is expected in the format: <osFamily>:<osVersion>:<osArch>, see components/os/descriptor.go
	DDInfraOSImageID                        = "osImageID"
	DDInfraOSImageIDUseLatest               = "osImageIDUseLatest"
	DDInfraDeployFakeintakeWithLoadBalancer = "deployFakeintakeWithLoadBalancer"
	DDInfraExtraResourcesTags               = "extraResourcesTags"
	DDInfraSSHUser                          = "sshUser"
	DDInfraInitOnly                         = "initOnly"
	DDInfraDialErrorLimit                   = "dialErrorLimit"
	DDInfraPerDialTimeoutSeconds            = "perDialTimeoutSeconds"

	// Agent Namespace
	DDAgentDeployParamName               = "deploy"
	DDAgentDeployWithOperatorParamName   = "deployWithOperator"
	DDAgentVersionParamName              = "version"
	DDAgentFlavorParamName               = "flavor"
	DDAgentPipelineID                    = "pipeline_id"
	DDAgentLocalPackage                  = "localPackage"
	DDAgentLocalChartPath                = "localChartPath"
	DDAgentCommitSHA                     = "commit_sha"
	DDAgentFullImagePathParamName        = "fullImagePath"
	DDClusterAgentVersionParamName       = "clusterAgentVersion"
	DDClusterAgentFullImagePathParamName = "clusterAgentFullImagePath"
	DDOperatorVersionParamName           = "operatorVersion"
	DDOperatorFullImagePathParamName     = "operatorFullImagePath"
	DDOperatorLocalChartPath             = "localChartPath"
	DDImagePullRegistryParamName         = "imagePullRegistry"
	DDImagePullUsernameParamName         = "imagePullUsername"
	DDImagePullPasswordParamName         = "imagePullPassword"
	DDAgentAPIKeyParamName               = "apiKey"
	DDAgentAPPKeyParamName               = "appKey"
	DDAgentFakeintake                    = "fakeintake"
	DDAgentDualShipping                  = "dualshipping"
	DDAGentFakeintakeRetentionPeriod     = "fakeintakeRetentionPeriod"
	DDAgentSite                          = "site"
	DDAgentMajorVersion                  = "majorVersion"
	DDAgentExtraEnvVars                  = "extraEnvVars" // extraEnvVars is expected in the format: <key1>=<value1>,<key2>=<value2>,...
	DDAgentJMX                           = "jmx"
	DDAgentFIPS                          = "fips"
	DDAgentLinuxOnly                     = "linuxOnly"
	DDAgentConfigPathParamName           = "configPath"
	DDAgentHelmConfig                    = "helmConfig"

	// Updater Namespace
	DDUpdaterParamName = "deploy"

	// Testing workload namerNamespace
	DDTestingWorkloadDeployParamName   = "deploy"
	DDTestingWorkloadDeployArgoRollout = "deployArgoRollout"

	// Dogstatsd namespace
	DDDogstatsdDeployParamName        = "deploy"
	DDDogstatsdFullImagePathParamName = "fullImagePath"

	DefaultMajorVersion = "7"
)

type CommonEnvironment struct {
	providerRegistry

	ctx         *pulumi.Context
	commonNamer namer.Namer

	InfraConfig           *sdkconfig.Config
	AgentConfig           *sdkconfig.Config
	TestingWorkloadConfig *sdkconfig.Config
	DogstatsdConfig       *sdkconfig.Config
	UpdaterConfig         *sdkconfig.Config
	OperatorConfig        *sdkconfig.Config

	username string
}

type Env interface {
	provider

	Ctx() *pulumi.Context
	CommonNamer() namer.Namer

	InfraShouldDeployFakeintakeWithLB() bool
	InfraEnvironmentNames() []string
	InfraOSDescriptor() string
	InfraOSImageID() string
	KubernetesVersion() string
	KubeNodeURL() string
	KindVersion() string
	DefaultResourceTags() map[string]string
	ExtraResourcesTags() map[string]string
	ResourcesTags() pulumi.StringMapInput
	AgentExtraEnvVars() map[string]string

	AgentDeploy() bool
	AgentVersion() string
	AgentFIPS() bool
	AgentLinuxOnly() bool
	AgentLocalPackage() string
	AgentLocalChartPath() string
	PipelineID() string
	CommitSHA() string
	ClusterAgentVersion() string
	AgentFullImagePath() string
	ClusterAgentFullImagePath() string
	OperatorFullImagePath() string
	OperatorVersion() string
	OperatorLocalChartPath() string
	ImagePullRegistry() string
	ImagePullUsername() string
	ImagePullPassword() pulumi.StringOutput
	AgentAPIKey() pulumi.StringOutput
	AgentAPPKey() pulumi.StringOutput
	AgentUseFakeintake() bool
	TestingWorkloadDeploy() bool
	InitOnly() bool
	DogstatsdDeploy() bool
	DogstatsdFullImagePath() string
	UpdaterDeploy() bool
	MajorVersion() string
	AgentHelmConfig() string

	GetBoolWithDefault(config *sdkconfig.Config, paramName string, defaultValue bool) bool
	GetStringListWithDefault(config *sdkconfig.Config, paramName string, defaultValue []string) []string
	GetStringWithDefault(config *sdkconfig.Config, paramName string, defaultValue string) string
	GetObjectWithDefault(config *sdkconfig.Config, paramName string, outputValue, defaultValue interface{}) interface{}
	GetIntWithDefault(config *sdkconfig.Config, paramName string, defaultValue int) int

	CloudEnv
}
type CloudEnv interface {
	// InternalDockerhubMirror returns the internal Dockerhub mirror.
	InternalDockerhubMirror() string
	// InternalRegistry returns the internal registry.
	InternalRegistry() string
	// InternalRegistryImageTagExists returns true if the image tag exists in the internal registry.
	InternalRegistryImageTagExists(image, tag string) (bool, error)
	// InternalRegistryFullImagePathExists returns true if the image and tag exists in the internal registry.
	InternalRegistryFullImagePathExists(fullImagePath string) (bool, error)
}

func NewCommonEnvironment(ctx *pulumi.Context) (CommonEnvironment, error) {
	env := CommonEnvironment{
		ctx:                   ctx,
		InfraConfig:           sdkconfig.New(ctx, DDInfraConfigNamespace),
		AgentConfig:           sdkconfig.New(ctx, DDAgentConfigNamespace),
		TestingWorkloadConfig: sdkconfig.New(ctx, DDTestingWorkloadNamespace),
		DogstatsdConfig:       sdkconfig.New(ctx, DDDogstatsdNamespace),
		UpdaterConfig:         sdkconfig.New(ctx, DDUpdaterConfigNamespace),
		commonNamer:           namer.NewNamer(ctx, ""),
		OperatorConfig:        sdkconfig.New(ctx, DDOperatorConfigNamespace),
		providerRegistry:      newProviderRegistry(ctx),
	}
	// store username
	user, err := user.Current()
	if err != nil {
		return env, err
	}
	env.username = strings.ReplaceAll(user.Username, "\\", "/")

	ctx.Log.Debug(fmt.Sprintf("user name: %s", env.username), nil)
	ctx.Log.Debug(fmt.Sprintf("resource tags: %v", env.DefaultResourceTags()), nil)
	ctx.Log.Debug(fmt.Sprintf("agent version: %s", env.AgentVersion()), nil)
	ctx.Log.Debug(fmt.Sprintf("pipeline id: %s", env.PipelineID()), nil)
	ctx.Log.Debug(fmt.Sprintf("deploy: %v", env.AgentDeploy()), nil)
	ctx.Log.Debug(fmt.Sprintf("full image path: %v", env.AgentFullImagePath()), nil)
	ctx.Log.Debug(fmt.Sprintf("deploy with Operator: %v", env.AgentDeployWithOperator()), nil)
	ctx.Log.Debug(fmt.Sprintf("operator full image path: %v", env.OperatorFullImagePath()), nil)
	return env, nil
}

func (e *CommonEnvironment) Ctx() *pulumi.Context {
	return e.ctx
}

func (e *CommonEnvironment) CommonNamer() namer.Namer {
	return e.commonNamer
}

// Infra namespace

func (e *CommonEnvironment) InfraShouldDeployFakeintakeWithLB() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraDeployFakeintakeWithLoadBalancer, true)
}

func (e *CommonEnvironment) InfraEnvironmentNames() []string {
	envsStr := e.InfraConfig.Require(DDInfraEnvironment)
	return strings.Split(envsStr, multiValueSeparator)
}

func (e *CommonEnvironment) InfraOSDescriptor() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraOSDescriptor, "")
}

func (e *CommonEnvironment) InfraOSImageID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraOSImageID, "")
}

func (e *CommonEnvironment) InfraOSImageIDUseLatest() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraOSImageIDUseLatest, false)
}

func (e *CommonEnvironment) KubernetesVersion() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraKubernetesVersion, "1.34")
}

func (e *CommonEnvironment) KindVersion() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraKindVersion, "v0.31.0")
}

func (e *CommonEnvironment) KubeNodeURL() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraKubeNodeURL, "")
}

func (e *CommonEnvironment) DefaultResourceTags() map[string]string {
	return map[string]string{"managed-by": "pulumi", "username": e.username}
}

func (e *CommonEnvironment) InitOnly() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraInitOnly, false)
}

func (e *CommonEnvironment) ExtraResourcesTags() map[string]string {
	tags, err := tagListToKeyValueMap(e.GetStringListWithDefault(e.InfraConfig, DDInfraExtraResourcesTags, []string{}))
	if err != nil {
		e.Ctx().Log.Error(fmt.Sprintf("error in extra resources tags : %v", err), nil)
	}
	return tags
}

func (e *CommonEnvironment) InfraSSHUser() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraSSHUser, "")
}

func (e *CommonEnvironment) InfraDialErrorLimit() int {
	return e.GetIntWithDefault(e.InfraConfig, DDInfraDialErrorLimit, 0)
}

func (e *CommonEnvironment) InfraPerDialTimeoutSeconds() int {
	return e.GetIntWithDefault(e.InfraConfig, DDInfraPerDialTimeoutSeconds, 0)
}

func EnvVariableResourceTags() map[string]string {
	tags := map[string]string{}
	lookupVars := []string{"TEAM", "PIPELINE_ID", "CI_PIPELINE_ID"}
	for _, varName := range lookupVars {
		if val := os.Getenv(varName); val != "" {
			tags[varName] = val
		}
	}
	return tags
}

func (e *CommonEnvironment) ResourcesTags() pulumi.StringMapInput {
	tags := pulumi.StringMap{}

	// default tags
	extendTagsMap(tags, e.DefaultResourceTags())
	// extended resource tags
	extendTagsMap(tags, e.ExtraResourcesTags())
	// env variable tags
	extendTagsMap(tags, EnvVariableResourceTags())

	return tags
}

// Agent Namespace
func (e *CommonEnvironment) AgentDeploy() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentDeployParamName, true)
}

func (e *CommonEnvironment) AgentDeployWithOperator() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentDeployWithOperatorParamName, false)
}

func (e *CommonEnvironment) AgentDeployArgoRollout() bool {
	return e.GetBoolWithDefault(e.TestingWorkloadConfig, DDTestingWorkloadDeployArgoRollout, false)
}

func (e *CommonEnvironment) AgentVersion() string {
	return e.AgentConfig.Get(DDAgentVersionParamName)
}

func (e *CommonEnvironment) AgentFlavor() string {
	return e.AgentConfig.Get(DDAgentFlavorParamName)
}

func (e *CommonEnvironment) AgentLocalPackage() string {
	return e.AgentConfig.Get(DDAgentLocalPackage)
}
func (e *CommonEnvironment) AgentLocalChartPath() string {
	return e.AgentConfig.Get(DDAgentLocalChartPath)
}
func (e *CommonEnvironment) PipelineID() string {
	return e.AgentConfig.Get(DDAgentPipelineID)
}

func (e *CommonEnvironment) CommitSHA() string {
	return e.AgentConfig.Get(DDAgentCommitSHA)
}

func (e *CommonEnvironment) ClusterAgentVersion() string {
	return e.AgentConfig.Get(DDClusterAgentVersionParamName)
}

func (e *CommonEnvironment) AgentFullImagePath() string {
	return e.AgentConfig.Get(DDAgentFullImagePathParamName)
}

func (e *CommonEnvironment) ClusterAgentFullImagePath() string {
	return e.AgentConfig.Get(DDClusterAgentFullImagePathParamName)
}

func (e *CommonEnvironment) OperatorVersion() string {
	return e.OperatorConfig.Get(DDOperatorVersionParamName)
}

func (e *CommonEnvironment) OperatorFullImagePath() string {
	return e.OperatorConfig.Get(DDOperatorFullImagePathParamName)
}
func (e *CommonEnvironment) OperatorLocalChartPath() string {
	return e.OperatorConfig.Get(DDOperatorLocalChartPath)
}
func (e *CommonEnvironment) ImagePullRegistry() string {
	return e.AgentConfig.Get(DDImagePullRegistryParamName)
}

func (e *CommonEnvironment) ImagePullUsername() string {
	return e.AgentConfig.Require(DDImagePullUsernameParamName)
}

func (e *CommonEnvironment) ImagePullPassword() pulumi.StringOutput {
	return e.AgentConfig.RequireSecret(DDImagePullPasswordParamName)
}

func (e *CommonEnvironment) AgentAPIKey() pulumi.StringOutput {
	return e.AgentConfig.RequireSecret(DDAgentAPIKeyParamName)
}

func (e *CommonEnvironment) AgentAPPKey() pulumi.StringOutput {
	return e.AgentConfig.RequireSecret(DDAgentAPPKeyParamName)
}

func (e *CommonEnvironment) AgentUseFakeintake() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentFakeintake, true)
}

func (e *CommonEnvironment) AgentUseDualShipping() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentDualShipping, false)
}

func (e *CommonEnvironment) AgentFakeintakeRetentionPeriod() string {
	return e.AgentConfig.Get(DDAGentFakeintakeRetentionPeriod)
}

func (e *CommonEnvironment) Site() string {
	return e.AgentConfig.Get(DDAgentSite)
}

func (e *CommonEnvironment) MajorVersion() string {
	return e.GetStringWithDefault(e.AgentConfig, DDAgentMajorVersion, DefaultMajorVersion)
}

func (e *CommonEnvironment) AgentExtraEnvVars() map[string]string {
	result := make(map[string]string)
	envVars, err := e.AgentConfig.Try(DDAgentExtraEnvVars)

	// If not found
	if err != nil {
		return result
	}

	extraEnvVarsList := strings.SplitSeq(strings.Trim(envVars, " "), ",")

	for envVar := range extraEnvVarsList {
		name, value, ok := strings.Cut(envVar, "=")
		if !ok {
			e.Ctx().Log.Warn(fmt.Sprintf("Invalid extraEnvVar format: %s", envVar), nil)
			continue
		}
		result[name] = value
	}
	return result
}

// Testing workload namespace
func (e *CommonEnvironment) TestingWorkloadDeploy() bool {
	return e.GetBoolWithDefault(e.TestingWorkloadConfig, DDTestingWorkloadDeployParamName, true)
}

// Dogstatsd namespace
func (e *CommonEnvironment) DogstatsdDeploy() bool {
	return e.GetBoolWithDefault(e.DogstatsdConfig, DDDogstatsdDeployParamName, true)
}

func (e *CommonEnvironment) DogstatsdFullImagePath() string {
	return e.AgentConfig.Get(DDDogstatsdFullImagePathParamName)
}

// Updater namespace
func (e *CommonEnvironment) UpdaterDeploy() bool {
	return e.GetBoolWithDefault(e.UpdaterConfig, DDUpdaterParamName, false)
}

// Generic methods
func (e *CommonEnvironment) GetBoolWithDefault(config *sdkconfig.Config, paramName string, defaultValue bool) bool {
	val, err := config.TryBool(paramName)
	if err == nil {
		return val
	}

	if !errors.Is(err, sdkconfig.ErrMissingVar) {
		e.Ctx().Log.Error(fmt.Sprintf("Parameter %s not parsable, err: %v, will use default value: %v", paramName, err, defaultValue), nil)
	}

	return defaultValue
}

func (e *CommonEnvironment) GetStringListWithDefault(config *sdkconfig.Config, paramName string, defaultValue []string) []string {
	val, err := config.Try(paramName)
	if err == nil {
		return strings.Split(val, multiValueSeparator)
	}

	if !errors.Is(err, sdkconfig.ErrMissingVar) {
		e.Ctx().Log.Error(fmt.Sprintf("Parameter %s not parsable, err: %v, will use default value: %v", paramName, err, defaultValue), nil)
	}

	return defaultValue
}

func (e *CommonEnvironment) GetStringWithDefault(config *sdkconfig.Config, paramName string, defaultValue string) string {
	val, err := config.Try(paramName)
	if err == nil {
		return val
	}

	if !errors.Is(err, sdkconfig.ErrMissingVar) {
		e.Ctx().Log.Error(fmt.Sprintf("Parameter %s not parsable, err: %v, will use default value: %v", paramName, err, defaultValue), nil)
	}

	return defaultValue
}

func (e *CommonEnvironment) GetObjectWithDefault(config *sdkconfig.Config, paramName string, outputValue, defaultValue interface{}) interface{} {
	err := config.TryObject(paramName, outputValue)
	if err == nil {
		return outputValue
	}

	if !errors.Is(err, sdkconfig.ErrMissingVar) {
		e.Ctx().Log.Error(fmt.Sprintf("Parameter %s not parsable, err: %v, will use default value: %v", paramName, err, defaultValue), nil)
	}

	return defaultValue
}

func (e *CommonEnvironment) GetIntWithDefault(config *sdkconfig.Config, paramName string, defaultValue int) int {
	val, err := config.TryInt(paramName)
	if err == nil {
		return val
	}

	if !errors.Is(err, sdkconfig.ErrMissingVar) {
		e.Ctx().Log.Error(fmt.Sprintf("Parameter %s not parsable, err: %v, will use default value: %v", paramName, err, defaultValue), nil)
	}

	return defaultValue
}

func (e *CommonEnvironment) AgentFIPS() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentFIPS, false)
}

func (e *CommonEnvironment) AgentLinuxOnly() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentLinuxOnly, true)
}

func (e *CommonEnvironment) AgentJMX() bool {
	return e.GetBoolWithDefault(e.AgentConfig, DDAgentJMX, false)
}

func (e *CommonEnvironment) AgentConfigPath() string {
	return e.AgentConfig.Get(DDAgentConfigPathParamName)
}

func (e *CommonEnvironment) CustomAgentConfig() (string, error) {
	configPath := e.AgentConfigPath()
	if configPath == "" {
		return "", fmt.Errorf("agent config path is empty")
	}

	config, err := os.ReadFile(configPath)

	return string(config), err
}

func (e *CommonEnvironment) AgentHelmConfig() string {
	return e.GetStringWithDefault(e.AgentConfig, DDAgentHelmConfig, "")
}
