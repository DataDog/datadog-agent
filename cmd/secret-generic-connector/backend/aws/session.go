// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
)

// SessionBackendConfig is the session configuration for AWS backends
type SessionBackendConfig struct {
	Region          string `mapstructure:"aws_region"`
	AccessKeyID     string `mapstructure:"aws_access_key_id"`
	SecretAccessKey string `mapstructure:"aws_secret_access_key"`
	Profile         string `mapstructure:"aws_profile"`
	RoleArn         string `mapstructure:"aws_role_arn"`
	ExternalID      string `mapstructure:"aws_external_id"`
}

// newConfigFromBackendConfig returns a resolved AWS configuration.
func newConfigFromBackendConfig(sessionConfig SessionBackendConfig) (*awsutil.AWSConfig, error) {
	return awsutil.ResolveConfig(context.TODO(), awsutil.SessionConfig{
		Region:          sessionConfig.Region,
		AccessKeyID:     sessionConfig.AccessKeyID,
		SecretAccessKey: sessionConfig.SecretAccessKey,
		Profile:         sessionConfig.Profile,
		RoleArn:         sessionConfig.RoleArn,
		ExternalID:      sessionConfig.ExternalID,
	})
}
