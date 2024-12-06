// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

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
	// PrivateKeyPassword private ssh key password
	PrivateKeyPassword StoreKey = "private_key_password"
	// PrivateKeyPath private ssh key path
	PrivateKeyPath StoreKey = "private_key_path"
	// Profile aws profile name
	Profile StoreKey = "profile"
	// PublicKeyPath public ssh key path
	PublicKeyPath StoreKey = "public_key_path"
	// PulumiPassword config file parameter name
	PulumiPassword StoreKey = "pulumi_password"
	// SkipDeleteOnFailure keep the stack on test failure
	SkipDeleteOnFailure StoreKey = "skip_delete_on_failure"
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
	// PreInitialized config flag parameter name
	PreInitialized StoreKey = "pre_initialized"
	// MajorVersion config flag parameter name
	MajorVersion StoreKey = "major_version"
)
