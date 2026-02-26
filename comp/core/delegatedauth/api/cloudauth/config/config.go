// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package config provides the config for specific delegated auth exchanges
package config

// ProviderAWS is the specifier for the AWS provider type
const ProviderAWS = "aws"

// AWSProviderConfig contains AWS-specific configuration for delegated auth.
// Implements common.ProviderConfig interface.
type AWSProviderConfig struct {
	// Region specifies the AWS region. If empty, auto-detects from EC2 metadata.
	Region string
}

// ProviderName returns the provider name for AWS.
func (c *AWSProviderConfig) ProviderName() string {
	return ProviderAWS
}
