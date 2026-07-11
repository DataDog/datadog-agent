// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ec2

package aws

import (
	"context"
	"os"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

// Standard AWS environment variable names. Only the non-ec2 build reads these directly; the
// ec2 build resolves credentials via environment-specific providers (see resolve_credentials_ec2.go).
const (
	awsAccessKeyIDEnvVar     = "AWS_ACCESS_KEY_ID"
	awsSecretAccessKeyEnvVar = "AWS_SECRET_ACCESS_KEY"
	awsSessionTokenEnvVar    = "AWS_SESSION_TOKEN"
)

// resolveCredentials returns static credentials from environment variables for non-ec2 builds.
// Only AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY (and optionally AWS_SESSION_TOKEN) are checked.
// IRSA and container credential sources are not supported in this build variant.
func (a *AWSAuth) resolveCredentials(_ context.Context, _ pkgconfigmodel.Reader) *creds.SecurityCredentials {
	accessKeyID := os.Getenv(awsAccessKeyIDEnvVar)
	secretAccessKey := os.Getenv(awsSecretAccessKeyEnvVar)
	if accessKeyID != "" && secretAccessKey != "" {
		return &creds.SecurityCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			Token:           os.Getenv(awsSessionTokenEnvVar),
		}
	}
	return &creds.SecurityCredentials{}
}
