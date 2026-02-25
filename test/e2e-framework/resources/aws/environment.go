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

	// DDInfraDefaultVPCIDParamName is the parameter name for the default VPC ID.
	DDInfraDefaultVPCIDParamName = "aws/defaultVPCID"
	// DDInfraDefaultSubnetsParamName is the parameter name for the default subnets.
	DDInfraDefaultSubnetsParamName = "aws/defaultSubnets"
	// DDInfraDefaultSecurityGroupsParamName is the parameter name for the default security groups.
	DDInfraDefaultSecurityGroupsParamName = "aws/defaultSecurityGroups"
	// DDInfraDefaultInstanceTypeParamName is the parameter name for the default instance type.
	DDInfraDefaultInstanceTypeParamName = "aws/defaultInstanceType"
	// DDInfraDefaultInstanceProfileParamName is the parameter name for the default instance profile.
	DDInfraDefaultInstanceProfileParamName = "aws/defaultInstanceProfile"
	// DDInfraDefaultARMInstanceTypeParamName is the parameter name for the default ARM instance type.
	DDInfraDefaultARMInstanceTypeParamName = "aws/defaultARMInstanceType"
	// DDInfraDefaultWindowsInstanceTypeParamName is the parameter name for the default Windows instance type.
	DDInfraDefaultWindowsInstanceTypeParamName = "aws/defaultWindowsInstanceType"
	// DDInfraDefaultKeyPairParamName is the parameter name for the default key pair.
	DDInfraDefaultKeyPairParamName = "aws/defaultKeyPairName"
	// DDinfraDefaultPublicKeyPath is the parameter name for the default public key path.
	DDinfraDefaultPublicKeyPath = "aws/defaultPublicKeyPath"
	// DDInfraDefaultPrivateKeyPath is the parameter name for the default private key path.
	DDInfraDefaultPrivateKeyPath = "aws/defaultPrivateKeyPath"
	// DDInfraDefaultPrivateKeyPassword is the parameter name for the default private key password.
	DDInfraDefaultPrivateKeyPassword = "aws/defaultPrivateKeyPassword"
	// DDInfraDefaultInstanceStorageSize is the parameter name for the default instance storage size.
	DDInfraDefaultInstanceStorageSize = "aws/defaultInstanceStorageSize"
	// DDInfraDefaultShutdownBehavior is the parameter name for the default shutdown behavior.
	DDInfraDefaultShutdownBehavior = "aws/defaultShutdownBehavior"
	// DDInfraDefaultInternalRegistry is the parameter name for the default internal registry.
	DDInfraDefaultInternalRegistry = "aws/defaultInternalRegistry"
	// DDInfraDefaultInternalDockerhubMirror is the parameter name for the default internal Docker Hub mirror.
	DDInfraDefaultInternalDockerhubMirror = "aws/defaultInternalDockerhubMirror"
	// DDInfraUseMacosCompatibleSubnets is the parameter name for macOS-compatible subnets.
	DDInfraUseMacosCompatibleSubnets = "aws/useMacosCompatibleSubnets"

	// DDInfraEcsExecKMSKeyID is the parameter name for the ECS exec KMS key ID.
	DDInfraEcsExecKMSKeyID = "aws/ecs/execKMSKeyID"
	// DDInfraEcsFargateFakeintakeClusterArns is the parameter name for the ECS Fargate fakeintake cluster ARNs.
	DDInfraEcsFargateFakeintakeClusterArns = "aws/ecs/fargateFakeintakeClusterArns"
	// DDInfraEcsFakeintakeLBs is the parameter name for the ECS fakeintake load balancers.
	DDInfraEcsFakeintakeLBs = "aws/ecs/defaultfakeintakeLBs"
	// DDInfraEcsTaskExecutionRole is the parameter name for the ECS task execution role.
	DDInfraEcsTaskExecutionRole = "aws/ecs/taskExecutionRole"
	// DDInfraEcsTaskRole is the parameter name for the ECS task role.
	DDInfraEcsTaskRole = "aws/ecs/taskRole"
	// DDInfraEcsInstanceProfile is the parameter name for the ECS instance profile.
	DDInfraEcsInstanceProfile = "aws/ecs/instanceProfile"
	// DDInfraEcsServiceAllocatePublicIP is the parameter name for allocating public IP to ECS services.
	DDInfraEcsServiceAllocatePublicIP = "aws/ecs/serviceAllocatePublicIP"
	// DDInfraEcsFargateCapacityProvider is the parameter name for the ECS Fargate capacity provider.
	DDInfraEcsFargateCapacityProvider = "aws/ecs/fargateCapacityProvider"
	// DDInfraEcsLinuxECSOptimizedNodeGroup is the parameter name for the Linux ECS-optimized node group.
	DDInfraEcsLinuxECSOptimizedNodeGroup = "aws/ecs/linuxECSOptimizedNodeGroup"
	// DDInfraEcsLinuxECSOptimizedARMNodeGroup is the parameter name for the Linux ECS-optimized ARM node group.
	DDInfraEcsLinuxECSOptimizedARMNodeGroup = "aws/ecs/linuxECSOptimizedARMNodeGroup"
	// DDInfraEcsLinuxBottlerocketNodeGroup is the parameter name for the Linux Bottlerocket node group.
	DDInfraEcsLinuxBottlerocketNodeGroup = "aws/ecs/linuxBottlerocketNodeGroup"
	// DDInfraEcsWindowsLTSCNodeGroup is the parameter name for the Windows LTSC node group.
	DDInfraEcsWindowsLTSCNodeGroup = "aws/ecs/windowsLTSCNodeGroup"

	// DDInfraEKSPODSubnets is the parameter name for EKS pod subnets.
	DDInfraEKSPODSubnets = "aws/eks/podSubnets"
	// DDInfraEksAllowedInboundSecurityGroups is the parameter name for EKS allowed inbound security groups.
	DDInfraEksAllowedInboundSecurityGroups = "aws/eks/inboundSecurityGroups"
	// DDInfraEksAllowedInboundPrefixList is the parameter name for EKS allowed inbound prefix lists.
	DDInfraEksAllowedInboundPrefixList = "aws/eks/inboundPrefixLists"
	// DDInfraEksAllowedInboundManagedPrefixListNames is the parameter name for EKS managed prefix list names.
	DDInfraEksAllowedInboundManagedPrefixListNames = "aws/eks/inboundManagedPrefixListNames"
	// DDInfraEksFargateNamespace is the parameter name for the EKS Fargate namespace.
	DDInfraEksFargateNamespace = "aws/eks/fargateNamespace"
	// DDInfraEksLinuxNodeGroup is the parameter name for the EKS Linux node group.
	DDInfraEksLinuxNodeGroup = "aws/eks/linuxNodeGroup"
	// DDInfraEksLinuxARMNodeGroup is the parameter name for the EKS Linux ARM node group.
	DDInfraEksLinuxARMNodeGroup = "aws/eks/linuxARMNodeGroup"
	// DDInfraEksLinuxBottlerocketNodeGroup is the parameter name for the EKS Linux Bottlerocket node group.
	DDInfraEksLinuxBottlerocketNodeGroup = "aws/eks/linuxBottlerocketNodeGroup"
	// DDInfraEksWindowsNodeGroup is the parameter name for the EKS Windows node group.
	DDInfraEksWindowsNodeGroup = "aws/eks/windowsNodeGroup"
	// DDInfraEksAccountAdminSSORole is the parameter name for the EKS account admin SSO role.
	DDInfraEksAccountAdminSSORole = "aws/eks/accountAdminSSORole"
	// DDInfraEksReadOnlySSORole is the parameter name for the EKS read-only SSO role.
	DDInfraEksReadOnlySSORole = "aws/eks/readOnlySSORole"
)

// Environment represents an AWS environment configuration for e2e tests.
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

// WithCommonEnvironment sets the common environment for the AWS environment.
func WithCommonEnvironment(e *config.CommonEnvironment) func(*Environment) {
	return func(awsEnv *Environment) {
		awsEnv.CommonEnvironment = e
	}
}

// NewEnvironment creates a new AWS environment with the given Pulumi context and options.
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

// InternalRegistry returns the internal container registry URL.
func (e *Environment) InternalRegistry() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInternalRegistry, e.envDefault.ddInfra.defaultInternalRegistry)
}

// InternalDockerhubMirror returns the internal Docker Hub mirror URL.
func (e *Environment) InternalDockerhubMirror() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInternalDockerhubMirror, e.envDefault.ddInfra.defaultInternalDockerhubMirror)
}

// InternalRegistryImageTagExists checks if the image exists in the internal registry.
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

// InternalRegistryFullImagePathExists checks if a full image path exists in the internal registry.
func (e *Environment) InternalRegistryFullImagePathExists(fullImagePath string) (bool, error) {
	image, tag := utils.ParseImageReference(fullImagePath)
	return e.InternalRegistryImageTagExists(image, tag)
}

// Region returns the AWS region.
func (e *Environment) Region() string {
	return e.GetStringWithDefault(e.awsConfig, awsRegionParamName, e.envDefault.aws.region)
}

// Profile returns the AWS profile name.
func (e *Environment) Profile() string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return ""
	}

	if profile := os.Getenv("AWS_PROFILE"); profile != "" {
		return profile
	}

	return e.GetStringWithDefault(e.awsConfig, awsProfileParamName, e.envDefault.aws.profile)
}

// DefaultVPCID returns the default VPC ID.
func (e *Environment) DefaultVPCID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultVPCIDParamName, e.envDefault.ddInfra.defaultVPCID)
}

// DefaultSubnets returns the default subnet IDs.
func (e *Environment) DefaultSubnets() []string {
	defaultSubnets := []string{}
	for _, subnet := range e.envDefault.ddInfra.defaultSubnets {
		if !e.UseMacosCompatibleSubnets() || subnet.MacOSCompatible {
			defaultSubnets = append(defaultSubnets, subnet.ID)
		}
	}
	return defaultSubnets
}

// DefaultFakeintakeECSArns returns the default fakeintake ECS cluster ARNs.
func (e *Environment) DefaultFakeintakeECSArns() []string {
	return e.GetStringListWithDefault(e.InfraConfig, DDInfraEcsFargateFakeintakeClusterArns, e.envDefault.ddInfra.ecs.fargateFakeintakeClusterArn)
}

// DefaultFakeintakeLBs returns the default fakeintake load balancer configurations.
func (e *Environment) DefaultFakeintakeLBs() []FakeintakeLBConfig {
	var fakeintakeLBConfig FakeintakeLBConfig
	return e.GetObjectWithDefault(e.InfraConfig, DDInfraEcsFakeintakeLBs, fakeintakeLBConfig, e.envDefault.ddInfra.ecs.defaultFakeintakeLBs).([]FakeintakeLBConfig)
}

// RandomSubnets returns a randomly shuffled subset of the default subnets.
func (e *Environment) RandomSubnets() pulumi.StringArrayOutput {
	return e.randomSubnets
}

// DefaultSecurityGroups returns the default security group IDs.
func (e *Environment) DefaultSecurityGroups() []string {
	return e.GetStringListWithDefault(e.InfraConfig, DDInfraDefaultSecurityGroupsParamName, e.envDefault.ddInfra.defaultSecurityGroups)
}

// DefaultInstanceType returns the default EC2 instance type.
func (e *Environment) DefaultInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceTypeParamName, e.envDefault.ddInfra.defaultInstanceType)
}

// DefaultInstanceProfileName returns the default IAM instance profile name.
func (e *Environment) DefaultInstanceProfileName() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceProfileParamName, e.envDefault.ddInfra.defaultInstanceProfileName)
}

// DefaultARMInstanceType returns the default ARM64 EC2 instance type.
func (e *Environment) DefaultARMInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultARMInstanceTypeParamName, e.envDefault.ddInfra.defaultARMInstanceType)
}

// DefaultWindowsInstanceType returns the default Windows EC2 instance type.
func (e *Environment) DefaultWindowsInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultWindowsInstanceTypeParamName, e.envDefault.ddInfra.defaultWindowsInstanceType)
}

// DefaultKeyPairName returns the default SSH key pair name.
func (e *Environment) DefaultKeyPairName() string {
	// No default value for keyPair
	return e.InfraConfig.Require(DDInfraDefaultKeyPairParamName)
}

// DefaultPublicKeyPath returns the path to the default SSH public key.
func (e *Environment) DefaultPublicKeyPath() string {
	return e.InfraConfig.Require(DDinfraDefaultPublicKeyPath)
}

// DefaultPrivateKeyPath returns the path to the default SSH private key.
func (e *Environment) DefaultPrivateKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPath)
}

// DefaultPrivateKeyPassword returns the password for the default SSH private key.
func (e *Environment) DefaultPrivateKeyPassword() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPassword)
}

// DefaultInstanceStorageSize returns the default instance storage size in GB.
func (e *Environment) DefaultInstanceStorageSize() int {
	return e.GetIntWithDefault(e.InfraConfig, DDInfraDefaultInstanceStorageSize, e.envDefault.ddInfra.defaultInstanceStorageSize)
}

// DefaultShutdownBehavior returns the default shutdown behavior ('terminate' or 'stop').
func (e *Environment) DefaultShutdownBehavior() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultShutdownBehavior, e.envDefault.ddInfra.defaultShutdownBehavior)
}

// UseMacosCompatibleSubnets returns whether to use macOS-compatible subnets.
func (e *Environment) UseMacosCompatibleSubnets() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraUseMacosCompatibleSubnets, e.envDefault.ddInfra.useMacosCompatibleSubnets)
}

// ECSExecKMSKeyID returns the KMS key ID used for ECS exec.
func (e *Environment) ECSExecKMSKeyID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsExecKMSKeyID, e.envDefault.ddInfra.ecs.execKMSKeyID)
}

// ECSFargateFakeintakeClusterArn returns a random fakeintake ECS cluster ARN.
func (e *Environment) ECSFargateFakeintakeClusterArn() pulumi.StringOutput {
	return e.randomECSArn
}

// ECSFakeintakeLBListenerArn returns a random fakeintake load balancer listener ARN.
func (e *Environment) ECSFakeintakeLBListenerArn() pulumi.StringOutput {
	defaultFakeintakeLBListenerArns := []string{}
	for _, fakeintake := range e.DefaultFakeintakeLBs() {
		defaultFakeintakeLBListenerArns = append(defaultFakeintakeLBListenerArns, fakeintake.listenerArn)
	}

	return pulumi.ToStringArray(defaultFakeintakeLBListenerArns).ToStringArrayOutput().Index(e.randomLBIdx)
}

// ECSFakeintakeLBBaseHost returns a random fakeintake load balancer base host.
func (e *Environment) ECSFakeintakeLBBaseHost() pulumi.StringOutput {
	defaultFakeintakeLBBaseHost := []string{}
	for _, fakeintake := range e.DefaultFakeintakeLBs() {
		defaultFakeintakeLBBaseHost = append(defaultFakeintakeLBBaseHost, fakeintake.baseHost)
	}

	return pulumi.ToStringArray(defaultFakeintakeLBBaseHost).ToStringArrayOutput().Index(e.randomLBIdx)
}

// ECSTaskExecutionRole returns the ECS task execution role ARN.
func (e *Environment) ECSTaskExecutionRole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsTaskExecutionRole, e.envDefault.ddInfra.ecs.taskExecutionRole)
}

// ECSTaskRole returns the ECS task role ARN.
func (e *Environment) ECSTaskRole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsTaskRole, e.envDefault.ddInfra.ecs.taskRole)
}

// ECSInstanceProfile returns the ECS instance profile ARN.
func (e *Environment) ECSInstanceProfile() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEcsInstanceProfile, e.envDefault.ddInfra.ecs.instanceProfile)
}

// ECSServicePublicIP returns whether ECS services should allocate public IPs.
func (e *Environment) ECSServicePublicIP() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsServiceAllocatePublicIP, e.envDefault.ddInfra.ecs.serviceAllocatePublicIP)
}

// ECSFargateCapacityProvider returns whether the Fargate capacity provider is enabled.
func (e *Environment) ECSFargateCapacityProvider() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsFargateCapacityProvider, e.envDefault.ddInfra.ecs.fargateCapacityProvider)
}

// ECSLinuxECSOptimizedNodeGroup returns whether the Linux ECS-optimized node group is enabled.
func (e *Environment) ECSLinuxECSOptimizedNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxECSOptimizedNodeGroup, e.envDefault.ddInfra.ecs.linuxECSOptimizedNodeGroup)
}

// ECSLinuxECSOptimizedARMNodeGroup returns whether the Linux ECS-optimized ARM node group is enabled.
func (e *Environment) ECSLinuxECSOptimizedARMNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxECSOptimizedARMNodeGroup, e.envDefault.ddInfra.ecs.linuxECSOptimizedARMNodeGroup)
}

// ECSLinuxBottlerocketNodeGroup returns whether the Linux Bottlerocket node group is enabled.
func (e *Environment) ECSLinuxBottlerocketNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsLinuxBottlerocketNodeGroup, e.envDefault.ddInfra.ecs.linuxBottlerocketNodeGroup)
}

// ECSWindowsNodeGroup returns whether the Windows LTSC node group is enabled.
func (e *Environment) ECSWindowsNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEcsWindowsLTSCNodeGroup, e.envDefault.ddInfra.ecs.windowsLTSCNodeGroup)
}

// EKSPODSubnets returns the EKS pod subnet configurations.
func (e *Environment) EKSPODSubnets() []DDInfraEKSPodSubnets {
	var arr []DDInfraEKSPodSubnets
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEKSPODSubnets, arr, e.envDefault.ddInfra.eks.podSubnets)
	return resObj.([]DDInfraEKSPodSubnets)
}

// EKSAllowedInboundSecurityGroups returns the allowed inbound security groups for EKS.
func (e *Environment) EKSAllowedInboundSecurityGroups() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundSecurityGroups, arr, e.envDefault.ddInfra.eks.allowedInboundSecurityGroups)
	return resObj.([]string)
}

// EKSAllowedInboundPrefixLists returns the allowed inbound prefix lists for EKS.
func (e *Environment) EKSAllowedInboundPrefixLists() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundPrefixList, arr, e.envDefault.ddInfra.eks.allowedInboundPrefixList)
	return resObj.([]string)
}

// EKSAllowedInboundManagedPrefixListNames returns the allowed inbound managed prefix list names for EKS.
func (e *Environment) EKSAllowedInboundManagedPrefixListNames() []string {
	var arr []string
	resObj := e.GetObjectWithDefault(e.InfraConfig, DDInfraEksAllowedInboundManagedPrefixListNames, arr, e.envDefault.ddInfra.eks.allowedInboundManagedPrefixListNames)
	return resObj.([]string)
}

// EKSFargateNamespace returns the EKS Fargate namespace.
func (e *Environment) EKSFargateNamespace() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksFargateNamespace, e.envDefault.ddInfra.eks.fargateNamespace)
}

// EKSLinuxNodeGroup returns whether the EKS Linux node group is enabled.
func (e *Environment) EKSLinuxNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxNodeGroup, e.envDefault.ddInfra.eks.linuxNodeGroup)
}

// EKSLinuxARMNodeGroup returns whether the EKS Linux ARM node group is enabled.
func (e *Environment) EKSLinuxARMNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxARMNodeGroup, e.envDefault.ddInfra.eks.linuxARMNodeGroup)
}

// EKSBottlerocketNodeGroup returns whether the EKS Bottlerocket node group is enabled.
func (e *Environment) EKSBottlerocketNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksLinuxBottlerocketNodeGroup, e.envDefault.ddInfra.eks.linuxBottlerocketNodeGroup)
}

// EKSWindowsNodeGroup returns whether the EKS Windows node group is enabled.
func (e *Environment) EKSWindowsNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEksWindowsNodeGroup, e.envDefault.ddInfra.eks.windowsLTSCNodeGroup)
}

// EKSAccountAdminSSORole returns the EKS account admin SSO role ARN.
func (e *Environment) EKSAccountAdminSSORole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksAccountAdminSSORole, e.envDefault.ddInfra.eks.accountAdminSSORole)
}

// EKSReadOnlySSORole returns the EKS read-only SSO role ARN.
func (e *Environment) EKSReadOnlySSORole() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraEksReadOnlySSORole, e.envDefault.ddInfra.eks.readOnlySSORole)
}

// GetCommonEnvironment returns the underlying common environment.
func (e *Environment) GetCommonEnvironment() *config.CommonEnvironment {
	return e.CommonEnvironment
}
