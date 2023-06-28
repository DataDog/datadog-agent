// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

type StoreKey string

const (
	Profile             StoreKey = "profile"
	APIKey              StoreKey = "api_key"
	APPKey              StoreKey = "app_key"
	PulumiPassword      StoreKey = "pulumi_password"
	StackParameters     StoreKey = "stack_params"
	SkipDeleteOnFailure StoreKey = "skip_delete_on_failure"
	KeyPairName         StoreKey = "key_pair_name"
	PublicKeyPath       StoreKey = "public_key_path"
	PrivateKeyPath      StoreKey = "private_key_path"
)
