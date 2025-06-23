// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import "github.com/DataDog/test-infra-definitions/components"

// StoreKey alias to string
type StoreKey string

const (
	// APIKey Datadog api key
	APIKey StoreKey = "api_key"
	// APPKey Datadog app key
	APPKey StoreKey = "app_key"
	// Environments space-separated cloud environments
	Environments StoreKey = "env"
	// ExtraResourcesTags extra tags to label resources
	ExtraResourcesTags StoreKey = "extra_resources_tags"
	// KeyPairName aws keypairname, used to access EC2 instances
	KeyPairName StoreKey = "key_pair_name"
	// AWSPrivateKeyPassword private ssh key password
	AWSPrivateKeyPassword StoreKey = StoreKey(components.CloudProviderAWS + PrivateKeyPasswordSuffix)
	// AWSPrivateKeyPath private ssh key path
	AWSPrivateKeyPath StoreKey = StoreKey(components.CloudProviderAWS + PrivateKeyPathSuffix)
	// Profile aws profile name
	Profile StoreKey = "profile"
	// AWSPublicKeyPath public ssh key path
	AWSPublicKeyPath StoreKey = StoreKey(components.CloudProviderAWS + PublicKeyPathSuffix)
	//AzurePrivateKeyPassword private ssh key password
	AzurePrivateKeyPassword StoreKey = StoreKey(components.CloudProviderAzure + PrivateKeyPasswordSuffix)
	//AzurePrivateKeyPath private ssh key path
	AzurePrivateKeyPath StoreKey = StoreKey(components.CloudProviderAzure + PrivateKeyPathSuffix)
	//AzurePublicKeyPath public ssh key path
	AzurePublicKeyPath StoreKey = StoreKey(components.CloudProviderAzure + PublicKeyPathSuffix)
	//GCPPrivateKeyPassword private ssh key password
	GCPPrivateKeyPassword StoreKey = StoreKey(components.CloudProviderGCP + PrivateKeyPasswordSuffix)
	//GCPPrivateKeyPath private ssh key path
	GCPPrivateKeyPath StoreKey = StoreKey(components.CloudProviderGCP + PrivateKeyPathSuffix)
	//GCPPublicKeyPath public ssh key path
	GCPPublicKeyPath StoreKey = StoreKey(components.CloudProviderGCP + PublicKeyPathSuffix)
	// LocalPublicKeyPath public ssh key path
	LocalPublicKeyPath StoreKey = "local_public_key_path"
	// PulumiPassword config file parameter name
	PulumiPassword StoreKey = "pulumi_password"
	// SkipDeleteOnFailure keep the stack on test failure
	SkipDeleteOnFailure StoreKey = "skip_delete_on_failure"
	// StackNameSuffix suffix to add to the stack name
	StackNameSuffix StoreKey = "stack_name_suffix"
	// StackParameters configuration map for the stack, in a json formatted string
	StackParameters StoreKey = "stack_params"
	// PipelineID  used to deploy agent artifacts from a Gitlab pipeline
	PipelineID StoreKey = "pipeline_id"
	// CommitSHA is used to deploy agent artifacts from a specific commit, needed for docker images
	CommitSHA StoreKey = "commit_sha"
	// VerifyCodeSignature of the agent
	VerifyCodeSignature StoreKey = "verify_code_signature"
	// OutputDir path to store test artifacts
	OutputDir StoreKey = "output_dir"
	// PulumiLogLevel sets the log level for pulumi. Pulumi emits logs at log levels between 1 and 11, with 11 being the most verbose.
	PulumiLogLevel StoreKey = "pulumi_log_level"
	// PulumiLogToStdErr specifies that all logs should be sent directly to stderr - making it more accessible and avoiding OS level buffering.
	PulumiLogToStdErr StoreKey = "pulumi_log_to_stderr"
	// PulumiVerboseProgressStreams allows specifying one or more io.Writers to redirect incremental update stdout
	PulumiVerboseProgressStreams StoreKey = "pulumi_verbose_progress_streams"
	// DevMode allows to keep the stack after the test completes
	DevMode StoreKey = "dev_mode"
	// InitOnly config flag parameter name
	InitOnly StoreKey = "init_only"
	// TeardownOnly config flag parameter name
	TeardownOnly StoreKey = "teardown_only"
	// PreInitialized config flag parameter name
	PreInitialized StoreKey = "pre_initialized"
	// MajorVersion config flag parameter name
	MajorVersion StoreKey = "major_version"
	// FIPS config flag parameter name
	FIPS StoreKey = "fips"
)

const (
	// PrivateKeyPathSuffix private ssh key path suffix
	PrivateKeyPathSuffix = "_private_key_path"
	// PublicKeyPathSuffix public ssh key path suffix
	PublicKeyPathSuffix = "_public_key_path"
	// PrivateKeyPasswordSuffix private ssh key password suffix
	PrivateKeyPasswordSuffix = "_private_key_password"
)
