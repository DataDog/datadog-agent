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

type AwsSessionBackendConfig struct {
	AwsRegion          string `mapstructure:"aws_region"`
	AwsAccessKeyId     string `mapstructure:"aws_access_key_id"`
	AwsSecretAccessKey string `mapstructure:"aws_secret_access_key"`
	AwsProfile         string `mapstructure:"aws_profile"`
	AwsRoleArn         string `mapstructure:"aws_role_arn"`
	AwsExternalId      string `mapstructure:"aws_external_id"`
}

func NewAwsConfigFromBackendConfig(backendId string, sessionConfig AwsSessionBackendConfig) (
	*aws.Config, error) {

	/* add LoadDefaultConfig support for:
	- SharedConfigFiles
	- SharedCredentialFiles
	*/

	// build slice of LoadOptionsFunc for LoadDefaultConfig overrides
	options := make([]func(*config.LoadOptions) error, 0)

	// aws_region
	if sessionConfig.AwsRegion != "" {
		options = append(options, func(o *config.LoadOptions) error {
			o.Region = sessionConfig.AwsRegion
			return nil
		})
	}

	// StaticCredentials (aws_access_key_id & aws_secret_access_key)
	if sessionConfig.AwsAccessKeyId != "" {
		if sessionConfig.AwsSecretAccessKey != "" {
			options = append(options, func(o *config.LoadOptions) error {
				o.Credentials = credentials.StaticCredentialsProvider{
					Value: aws.Credentials{
						AccessKeyID:     sessionConfig.AwsAccessKeyId,
						SecretAccessKey: sessionConfig.AwsSecretAccessKey,
					},
				}
				return nil
			})
		}
	}

	// SharedConfigProfile (aws_profile)
	if sessionConfig.AwsProfile != "" {
		options = append(options, config.WithSharedConfigProfile(sessionConfig.AwsProfile))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)

	// sts:AssumeRole (aws_role_arn)
	if sessionConfig.AwsRoleArn != "" {
		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), sessionConfig.AwsRoleArn,
			func(o *stscreds.AssumeRoleOptions) {
				if sessionConfig.AwsExternalId != "" {
					o.ExternalID = &sessionConfig.AwsExternalId
				}
			},
		)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return &cfg, err
}
