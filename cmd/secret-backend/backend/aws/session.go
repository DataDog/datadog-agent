// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	// log "github.com/sirupsen/logrus"
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

// newConfigFromBackendConfig returns a new config for AWS
func newConfigFromBackendConfig(sessionConfig SessionBackendConfig) (*aws.Config, error) {
	// build slice of LoadOptionsFunc for LoadDefaultConfig overrides
	options := make([]func(*config.LoadOptions) error, 0)

	// aws_region
	if sessionConfig.Region != "" {
		options = append(options, func(o *config.LoadOptions) error {
			o.Region = sessionConfig.Region
			return nil
		})
	}

	// StaticCredentials (aws_access_key_id & aws_secret_access_key)
	if sessionConfig.AccessKeyID != "" {
		if sessionConfig.SecretAccessKey != "" {
			options = append(options, func(o *config.LoadOptions) error {
				o.Credentials = credentials.StaticCredentialsProvider{
					Value: aws.Credentials{
						AccessKeyID:     sessionConfig.AccessKeyID,
						SecretAccessKey: sessionConfig.SecretAccessKey,
					},
				}
				return nil
			})
		}
	}

	// SharedConfigProfile (aws_profile)
	if sessionConfig.Profile != "" {
		options = append(options, config.WithSharedConfigProfile(sessionConfig.Profile))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)

	// sts:AssumeRole (aws_role_arn)
	if sessionConfig.RoleArn != "" {
		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), sessionConfig.RoleArn,
			func(o *stscreds.AssumeRoleOptions) {
				if sessionConfig.ExternalID != "" {
					o.ExternalID = &sessionConfig.ExternalID
				}
			},
		)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return &cfg, err
}
