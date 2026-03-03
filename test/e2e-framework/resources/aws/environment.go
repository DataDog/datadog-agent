// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	awsECR "github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	sdkaws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdkconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	awsConfigNamespace  = "aws"
	awsRegionParamName  = "region"
	awsProfileParamName = "profile"

	// AWS Infra
	DDInfraDefaultVPCIDParamName           = "aws/defaultVPCID"
	DDInfraDefaultSubnetsParamName         = "aws/defaultSubnets"
	DDInfraDefaultSecurityGroupsParamName  = "aws/defaultSecurityGroups"
	DDInfraDefaultInstanceTypeParamName        = "aws/defaultInstanceType"
	DDInfraDefaultInstanceProfileParamName     = "aws/defaultInstanceProfile"
	DDInfraDefaultARMInstanceTypeParamName     = "aws/defaultARMInstanceType"
	DDInfraDefaultWindowsInstanceTypeParamName = "aws/defaultWindowsInstanceType"
	DDInfraDefaultKeyPairParamName         = "aws/defaultKeyPairName"
	DDinfraDefaultPublicKeyPath            = "aws/defaultPublicKeyPath"
	DDInfraDefaultPrivateKeyPath           = "aws/defaultPrivateKeyPath"
	DDInfraDefaultPrivateKeyPassword       = "aws/defaultPrivateKeyPassword"
	DDInfraDefaultInstanceStorageSize      = "aws/defaultInstanceStorageSize"
	DDInfraDefaultShutdownBehavior         = "aws/defaultShutdownBehavior"
	DDInfraDefaultInternalRegistry         = "aws/defaultInternalRegistry"
	DDInfraDefaultInternalDockerhubMirror  = "aws/defaultInternalDockerhubMirror"
	DDInfraUseMacosCompatibleSubnets       = "aws/useMacosCompatibleSubnets"

	// AWS ECS
	DDInfraEcsExecKMSKeyID                  = "aws/ecs/execKMSKeyID"
	DDInfraEcsFargateFakeintakeClusterArns  = "aws/ecs/fargateFakeintakeClusterArns"
	DDInfraEcsFakeintakeLBs                 = "aws/ecs/defaultfakeintakeLBs"
	DDInfraEcsTaskExecutionRole             = "aws/ecs/taskExecutionRole"
	DDInfraEcsTaskRole                      = "aws/ecs/taskRole"
	DDInfraEcsInstanceProfile               = "aws/ecs/instanceProfile"
	DDInfraEcsServiceAllocatePublicIP       = "aws/ecs/serviceAllocatePublicIP"
	DDInfraEcsFargateCapacityProvider       = "aws/ecs/fargateCapacityProvider"
	DDInfraEcsLinuxECSOptimizedNodeGroup    = "aws/ecs/linuxECSOptimizedNodeGroup"
	DDInfraEcsLinuxECSOptimizedARMNodeGroup = "aws/ecs/linuxECSOptimizedARMNodeGroup"
	DDInfraEcsLinuxBottlerocketNodeGroup    = "aws/ecs/linuxBottlerocketNodeGroup"
	DDInfraEcsWindowsLTSCNodeGroup          = "aws/ecs/windowsLTSCNodeGroup"

	// AWS EKS
	DDInfraEKSPODSubnets                           = "aws/eks/podSubnets"
	DDInfraEksAllowedInboundSecurityGroups         = "aws/eks/inboundSecurityGroups"
	DDInfraEksAllowedInboundPrefixList             = "aws/eks/inboundPrefixLists"
	DDInfraEksAllowedInboundManagedPrefixListNames = "aws/eks/inboundManagedPrefixListNames"
	DDInfraEksFargateNamespace                     = "aws/eks/fargateNamespace"
	DDInfraEksLinuxNodeGroup                       = "aws/eks/linuxNodeGroup"
	DDInfraEksLinuxARMNodeGroup                    = "aws/eks/linuxARMNodeGroup"
	DDInfraEksLinuxBottlerocketNodeGroup           = "aws/eks/linuxBottlerocketNodeGroup"
	DDInfraEksWindowsNodeGroup                     = "aws/eks/windowsNodeGroup"
	DDInfraEksGPUNodeGroup                         = "aws/eks/gpuNodeGroup"
	DDInfraEksGPUInstanceType                      = "aws/eks/gpuInstanceType"
	DDInfraEksAccountAdminSSORole                  = "aws/eks/accountAdminSSORole"
	DDInfraEksReadOnlySSORole                      = "aws/eks/readOnlySSORole"
)

type Environment struct {
	*config.CommonEnvironment

	Namer namer.Namer

	awsConfig  *sdkconfig.Config
	envDefault environmentDefault

	randomSubnets pulumi.StringArrayOutput
	randomLBIdx   pulumi.IntOutput
	randomECSArn  pulumi.StringOutput
}

var registryIDCheck, _ = regexp.Compile("^[0-9]{12}")

var _ config.Env = (*Environment)(nil)

func WithCommonEnvironment(e *config.CommonEnvironment) func(*Environment) {
	return func(awsEnv *Environment) {
		awsEnv.CommonEnvironment = e
	}
}

func NewEnvironment(ctx *pulumi.Context, options ...func(*Environment)) (Environment, error) {
	env := Environment{
		Namer:     namer.NewNamer(ctx, awsConfigNamespace),
		awsConfig: sdkconfig.New(ctx, awsConfigNamespace),
	}

	for _, opt := range options {
		opt(&env)
	}

	if env.CommonEnvironment == nil {
		commonEnv, err := config.NewCommonEnvironment(ctx)
		if err != nil {
			return Environment{}, err
		}

		env.CommonEnvironment = &commonEnv
	}
	env.envDefault = getEnvironmentDefault(config.FindEnvironmentName(env.InfraEnvironmentNames(), awsConfigNamespace))

	awsProvider, err := sdkaws.NewProvider(ctx, string(config.ProviderAWS), &sdkaws.ProviderArgs{
		Region:  pulumi.String(env.Region()),
		Profile: pulumi.String(env.Profile()),
		DefaultTags: sdkaws.ProviderDefaultTagsArgs{
			Tags: env.ResourcesTags(),
		},
		SkipCredentialsValidation: pulumi.BoolPtr(false),
		SkipMetadataApiCheck:      pulumi.BoolPtr(false),
	})
	if err != nil {
		return Environment{}, err
	}
	env.RegisterProvider(config.ProviderAWS, awsProvider)

	shuffle, err := random.NewRandomShuffle(env.Ctx(), env.Namer.ResourceName("rnd-subnet"), &random.RandomShuffleArgs{
		Inputs:      pulumi.ToStringArray(env.DefaultSubnets()),
		ResultCount: pulumi.IntPtr(2),
	}, env.WithProviders(config.ProviderRandom))
	if err != nil {
		return Environment{}, err
	}
	env.randomSubnets = shuffle.Results

	shuffleFakeintakeECS, err := random.NewRandomShuffle(env.Ctx(), env.Namer.ResourceName("rnd-ecs"), &random.RandomShuffleArgs{
		Inputs:      pulumi.ToStringArray(env.DefaultFakeintakeECSArns()),
		ResultCount: pulumi.IntPtr(1),
	}, env.WithProviders(config.ProviderRandom))
	if err != nil {
		return Environment{}, err
	}
	env.randomECSArn = shuffleFakeintakeECS.Results.Index(pulumi.Int(0))

	if len(env.DefaultFakeintakeLBs()) == 0 {
		return env, nil
	}

	shuffleLB, err := random.NewRandomInteger(env.Ctx(), env.Namer.ResourceName("rnd-fakeintake"), &random.RandomIntegerArgs{
		Min: pulumi.Int(0),
		Max: pulumi.Int(len(env.DefaultFakeintakeLBs()) - 1),
	}, env.WithProviders(config.ProviderRandom))
	if err != nil {
		return Environment{}, err
	}
	env.randomLBIdx = shuffleLB.Result

	return env, nil
}

// Cross Cloud Provider config
func (e *Environment) InternalRegistry() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInternalRegistry, e.envDefault.ddInfra.defaultInternalRegistry)
}

func (e *Environment) InternalDockerhubMirror() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInternalDockerhubMirror, e.envDefault.ddInfra.defaultInternalDockerhubMirror)
}

// Check if the image exists in the internal registry
func (e *Environment) InternalRegistryImageTagExists(image, tag string) (bool, error) {

	if !registryIDCheck.MatchString(image) {
		// Return true as most likely not an ECR Docker image
		return true, nil
	}

	cfg, err := awsConfig.LoadDefaultConfig(e.Ctx().Context(),
		awsConfig.WithRegion(e.Region()),
		awsConfig.WithSharedConfigProfile(e.Profile()),
	)
	if err != nil {
		return false, err
	}

	ecrClient := awsECR.NewFromConfig(cfg)
	_, err = ecrClient.BatchGetImage(e.Ctx().Context(), &awsECR.BatchGetImageInput{
		RegistryId:     &strings.Split(image, ".")[0],
		RepositoryName: &strings.Split(image, "/")[len(strings.Split(image, "/"))-1],
		ImageIds:       []types.ImageIdentifier{{ImageTag: &tag}},
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

func (e *Environment) InternalRegistryFullImagePathExists(fullImagePath string) (bool, error) {
	image, tag := utils.ParseImageReference(fullImagePath)
	return e.InternalRegistryImageTagExists(image, tag)
}

// Common
func (e *Environment) Region() string {
	return e.GetStringWithDefault(e.awsConfig, awsRegionParamName, e.envDefault.aws.region)
}

func (e *Environment) Profile() string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return ""
	}

	if profile := os.Getenv("AWS_PROFILE"); profile != "" {
		return profile
	}

	return e.GetStringWithDefault(e.awsConfig, awsProfileParamName, e.envDefault.aws.profile)
}

func (e *Environment) DefaultVPCID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultVPCIDParamName, e.envDefault.ddInfra.defaultVPCID)
}

func (e *Environment) DefaultSubnets() []string {
	defaultSubnets := []string{}
	for _, subnet := range e.envDefault.ddInfra.defaultSubnets {
		if !e.UseMacosCompatibleSubnets() || subnet.MacOSCompatible {
			defaultSubnets = append(defaultSubnets, subnet.ID)
		}
	}
	return defaultSubnets
}

func (e *Environment) DefaultFakeintakeECSArns() []string {
	return e.GetStringListWithDefault(e.InfraConfig, DDInfraEcsFargateFakeintakeClusterArns, e.envDefault.ddInfra.ecs.fargateFakeintakeClusterArn)
}

func (e *Environment) DefaultFakeintakeLBs() []FakeintakeLBConfig {
	var fakeintakeLBConfig FakeintakeLBConfig
	return e.GetObjectWithDefault(e.InfraConfig, DDInfraEcsFakeintakeLBs, fakeintakeLBConfig, e.envDefault.ddInfra.ecs.defaultFakeintakeLBs).([]FakeintakeLBConfig)
}

func (e *Environment) RandomSubnets() pulumi.StringArrayOutput {
	return e.randomSubnets
}

func (e *Environment) DefaultSecurityGroups() []string {
	return e.GetStringListWithDefault(e.InfraConfig, DDInfraDefaultSecurityGroupsParamName, e.envDefault.ddInfra.defaultSecurityGroups)
}

func (e *Environment) DefaultInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceTypeParamName, e.envDefault.ddInfra.defaultInstanceType)
}

func (e *Environment) DefaultInstanceProfileName() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceProfileParamName, e.envDefault.ddInfra.defaultInstanceProfileName)
}

func (e *Environment) DefaultARMInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultARMInstanceTypeParamName, e.envDefault.ddInfra.defaultARMInstanceType)
}

func (e *Environment) DefaultWindowsInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultWindowsInstanceTypeParamName, e.envDefault.ddInfra.defaultWindowsInstanceType)
}

func (e *Environment) DefaultKeyPairName() string {
	// No default value for keyPair
	return e.InfraConfig.Require(DDInfraDefaultKeyPairParamName)
}

func (e *Environment) DefaultPublicKeyPath() string {
	return e.InfraConfig.Require(DDinfraDefaultPublicKeyPath)
}

func (e *Environment) DefaultPrivateKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPath)
}

func (e *Environment) DefaultPrivateKeyPassword() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPassword)
}

func (e *Environment) DefaultInstanceStorageSize() int {
	return e.GetIntWithDefault(e.InfraConfig, DDInfraDefaultInstanceStorageSize, e.envDefault.ddInfra.defaultInstanceStorageSize)
}

// shutdown behavior can be 'terminate' or 'stop'
func (e *Environment) DefaultShutdownBehavior() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultShutdownBehavior, e.envDefault.ddInfra.defaultShutdownBehavior)
}

func (e *Environment) UseMacosCompatibleSubnets() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraUseMacosCompatibleSubnets, e.envDefault.ddInfra.useMacosCompatibleSubnets)
}

// ECS
func (e *Environment) ECSExecKMSKeyID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsExecKMSKeyID, e.envDefault.ddInfra.ecs.execKMSKeyID)
}

func (e *Environment) ECSFargateFakeintakeClusterArn() pulumi.StringOutput {
	return e.randomECSArn
}

func (e *Environment) ECSFakeintakeLBListenerArn() pulumi.StringOutput {
	defaultFakeintakeLBListenerArns := []string{}
	for _, fakeintake := range e.DefaultFakeintakeLBs() {
		defaultFakeintakeLBListenerArns = append(defaultFakeintakeLBListenerArns, fakeintake.listenerArn)
	}

	return pulumi.ToStringArray(defaultFakeintakeLBListenerArns).ToStringArrayOutput().Index(e.randomLBIdx)
}

func (e *Environment) ECSFakeintakeLBBaseHost() pulumi.StringOutput {
	defaultFakeintakeLBBaseHost := []string{}
	for _, fakeintake := range e.DefaultFakeintakeLBs() {
		defaultFakeintakeLBBaseHost = append(defaultFakeintakeLBBaseHost, fakeintake.baseHost)
	}

	return pulumi.ToStringArray(defaultFakeintakeLBBaseHost).ToStringArrayOutput().Index(e.randomLBIdx)
}

func (e *Environment) ECSTaskExecutionRole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsTaskExecutionRole, e.envDefault.ddInfra.ecs.taskExecutionRole)
}

func (e *Environment) ECSTaskRole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsTaskRole, e.envDefault.ddInfra.ecs.taskRole)
}

func (e *Environment) ECSInstanceProfile() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsInstanceProfile, e.envDefault.ddInfra.ecs.instanceProfile)
}

func (e *Environment) ECSServicePublicIP() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsServiceAllocatePublicIP, e.envDefault.ddInfra.ecs.serviceAllocatePublicIP)
}

func (e *Environment) ECSFargateCapacityProvider() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsFargateCapacityProvider, e.envDefault.ddInfra.ecs.fargateCapacityProvider)
}

func (e *Environment) ECSLinuxECSOptimizedNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxECSOptimizedNodeGroup, e.envDefault.ddInfra.ecs.linuxECSOptimizedNodeGroup)
}

func (e *Environment) ECSLinuxECSOptimizedARMNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxECSOptimizedARMNodeGroup, e.envDefault.ddInfra.ecs.linuxECSOptimizedARMNodeGroup)
}

func (e *Environment) ECSLinuxBottlerocketNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxBottlerocketNodeGroup, e.envDefault.ddInfra.ecs.linuxBottlerocketNodeGroup)
}

func (e *Environment) ECSWindowsNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsWindowsLTSCNodeGroup, e.envDefault.ddInfra.ecs.windowsLTSCNodeGroup)
}

func (e *Environment) EKSPODSubnets() []DDInfraEKSPodSubnets {
	var arr []DDInfraEKSPodSubnets
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEKSPODSubnets, arr, e.envDefault.ddInfra.eks.podSubnets)
	return resObj.([]DDInfraEKSPodSubnets)
}

func (e *Environment) EKSAllowedInboundSecurityGroups() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundSecurityGroups, arr, e.envDefault.ddInfra.eks.allowedInboundSecurityGroups)
	return resObj.([]string)
}

func (e *Environment) EKSAllowedInboundPrefixLists() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundPrefixList, arr, e.envDefault.ddInfra.eks.allowedInboundPrefixList)
	return resObj.([]string)
}

func (e *Environment) EKSAllowedInboundManagedPrefixListNames() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundManagedPrefixListNames, arr, e.envDefault.ddInfra.eks.allowedInboundManagedPrefixListNames)
	return resObj.([]string)
}

func (e *Environment) EKSFargateNamespace() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksFargateNamespace, e.envDefault.ddInfra.eks.fargateNamespace)
}

func (e *Environment) EKSLinuxNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxNodeGroup, e.envDefault.ddInfra.eks.linuxNodeGroup)
}

func (e *Environment) EKSLinuxARMNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxARMNodeGroup, e.envDefault.ddInfra.eks.linuxARMNodeGroup)
}

func (e *Environment) EKSBottlerocketNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxBottlerocketNodeGroup, e.envDefault.ddInfra.eks.linuxBottlerocketNodeGroup)
}

func (e *Environment) EKSWindowsNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksWindowsNodeGroup, e.envDefault.ddInfra.eks.windowsLTSCNodeGroup)
}

func (e *Environment) EKSGPUNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksGPUNodeGroup, e.envDefault.ddInfra.eks.gpuNodeGroup)
}

func (e *Environment) EKSGPUInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksGPUInstanceType, e.envDefault.ddInfra.eks.gpuInstanceType)
}

func (e *Environment) EKSAccountAdminSSORole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksAccountAdminSSORole, e.envDefault.ddInfra.eks.accountAdminSSORole)
}

func (e *Environment) EKSReadOnlySSORole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksReadOnlySSORole, e.envDefault.ddInfra.eks.readOnlySSORole)
}
func (e *Environment) GetCommonEnvironment() *config.CommonEnvironment {
	return e.CommonEnvironment
}
