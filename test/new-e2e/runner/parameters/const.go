// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

// StoreKey exported type should have comment or be unexported
type StoreKey string

// This const block should have a comment or be unexported
const (
	APIKey              StoreKey = "api_key"
	APPKey              StoreKey = "app_key"
	Environments        StoreKey = "env"
	ExtraResourcesTags  StoreKey = "extra_resources_tags"
	KeyPairName         StoreKey = "key_pair_name"
	PrivateKeyPath      StoreKey = "private_key_path"
	Profile             StoreKey = "profile"
	PublicKeyPath       StoreKey = "public_key_path"
	PulumiPassword      StoreKey = "pulumi_password"
	SkipDeleteOnFailure StoreKey = "skip_delete_on_failure"
	StackParameters     StoreKey = "stack_params"
)
