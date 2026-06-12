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
// SDK's standard precedence. Behavior is otherwise governed by the standard AWS SDK
// environment variables (for example AWS_REGION, AWS_DEFAULT_REGION, AWS_PROFILE,
// AWS_EC2_METADATA_DISABLED).
//
// The one override: when delegated_auth.aws.region is configured (a.region), it is passed to
// the SDK so it takes precedence over AWS_REGION/AWS_DEFAULT_REGION, keeping credential
// resolution consistent with the signing endpoint. When it is unset, the SDK resolves the
// region from AWS_REGION/AWS_DEFAULT_REGION/IMDS itself, falling back to defaultRegion so a
// region always exists. This matters for IRSA-only pods that have AWS_ROLE_ARN and
// AWS_WEB_IDENTITY_TOKEN_FILE but no region set: the SDK's web-identity provider calls STS to
// retrieve credentials, and STS endpoint resolution needs a region. The fallback mirrors the
// signing path, which also defaults to defaultRegion when none is configured. The SDK handles
// caching and refresh internally.
func (a *AWSAuth) resolveCredentials(ctx context.Context) *creds.SecurityCredentials {
	cfg, err := config.LoadDefaultConfig(ctx, a.regionLoadOptions()...)
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

// regionLoadOptions builds the SDK region options. WithDefaultRegion only applies when no
// region is resolved from AWS_REGION/AWS_DEFAULT_REGION/IMDS, guaranteeing a region always
// exists (needed for the IRSA web-identity STS call); WithRegion, set when
// delegated_auth.aws.region is configured, takes precedence over it.
func (a *AWSAuth) regionLoadOptions() []func(*config.LoadOptions) error {
	opts := []func(*config.LoadOptions) error{config.WithDefaultRegion(defaultRegion)}
	if a.region != "" {
		opts = append(opts, config.WithRegion(a.region))
	}
	return opts
}
