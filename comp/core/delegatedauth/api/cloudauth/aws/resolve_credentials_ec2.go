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

// resolveCredentials uses the AWS SDK credential chain limited to:
// env vars -> web identity (IRSA) -> container (ECS/Pod Identity) -> EC2 IMDS.
// Shared config files, SSO, and credential_process are intentionally excluded.
// The SDK handles caching and refresh internally.
func (a *AWSAuth) resolveCredentials(ctx context.Context) *creds.SecurityCredentials {
	// Default the region the same way the signing code does. The web-identity
	// (IRSA) and container providers need a region to build the STS endpoint;
	// without one, credential retrieval fails before signing's own default applies.
	region := a.region
	if region == "" {
		region = defaultRegion
	}
	opts := []func(*config.LoadOptions) error{
		config.WithSharedConfigFiles([]string{}),
		config.WithSharedCredentialsFiles([]string{}),
		config.WithRegion(region),
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
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
