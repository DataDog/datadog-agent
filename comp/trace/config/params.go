// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// Params defines the parameters for the config component.
type Params struct {
	// FailIfAPIKeyMissing controls if the Agent should fail if the API key is missing from the config.
	FailIfAPIKeyMissing bool
}
