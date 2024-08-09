// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

// StoreKey alias to string
type StoreKey string

const (
	// APIKey config file parameter name
	APIKey StoreKey = "api_key"
	// APPKey config file parameter name
	APPKey StoreKey = "app_key"
	// Environments config file parameter name
	Environments StoreKey = "env"
	// ExtraResourcesTags config file parameter name
	ExtraResourcesTags StoreKey = "extra_resources_tags"
	// KeyPairName config file parameter name
	KeyPairName StoreKey = "key_pair_name"
	// PrivateKeyPassword config file parameter name
	PrivateKeyPassword StoreKey = "private_key_password"
	// PrivateKeyPath config file parameter name
	PrivateKeyPath StoreKey = "private_key_path"
	// Profile config file parameter name
	Profile StoreKey = "profile"
	// PublicKeyPath config file parameter name
	PublicKeyPath StoreKey = "public_key_path"
	// PulumiPassword config file parameter name
	PulumiPassword StoreKey = "pulumi_password"
	// SkipDeleteOnFailure config file parameter name
	SkipDeleteOnFailure StoreKey = "skip_delete_on_failure"
	// StackParameters config file parameter name
	StackParameters StoreKey = "stack_params"
	// PipelineID config file parameter name
	PipelineID StoreKey = "pipeline_id"
	// CommitSHA config file parameter name
	CommitSHA StoreKey = "commit_sha"
	// VerifyCodeSignature config file parameter name
	VerifyCodeSignature StoreKey = "verify_code_signature"
	// OutputDir config file parameter name
	OutputDir StoreKey = "output_dir"
	// PulumiLogLevel config file parameter name
	PulumiLogLevel StoreKey = "pulumi_log_level"
	// PulumiLogToStdErr config file parameter name
	PulumiLogToStdErr StoreKey = "pulumi_log_to_stderr"
	// PulumiVerboseProgressStreams config file parameter name
	PulumiVerboseProgressStreams StoreKey = "pulumi_verbose_progress_streams"
	// DevMode config flag parameter name
	DevMode StoreKey = "dev_mode"
	// InitOnly config flag parameter name
	InitOnly StoreKey = "init_only"
	// PreInitialized config flag parameter name
	PreInitialized StoreKey = "pre_initialized"
)
