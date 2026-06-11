// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resolveCredentials uses the AWS SDK default credential chain (ec2 build): environment
// variables, shared config and credentials files (named profiles, SSO, credential_process),
// web identity (IRSA), container credentials (ECS / EKS Pod Identity), and EC2 IMDS, in the
// SDK's standard precedence. Behavior is controlled entirely by the standard AWS SDK
// environment variables (for example AWS_REGION, AWS_DEFAULT_REGION, AWS_PROFILE,
// AWS_EC2_METADATA_DISABLED); no Datadog-specific overrides are applied. The SDK handles
// region resolution, caching, and refresh internally.
func (a *AWSAuth) resolveCredentials(ctx context.Context) *creds.SecurityCredentials {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Warnf("AWS SDK LoadDefaultConfig failed: %v", err)
		return &creds.SecurityCredentials{}
	}

	sdkCreds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		log.Warnf("AWS credential retrieval failed: %v", err)
		return &creds.SecurityCredentials{}
	}

	return &creds.SecurityCredentials{
		AccessKeyID:     sdkCreds.AccessKeyID,
		SecretAccessKey: sdkCreds.SecretAccessKey,
		Token:           sdkCreds.SessionToken,
	}
}
