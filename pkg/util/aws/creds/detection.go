// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package creds

import (
	"context"
	"os"

	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

// HasAWSCredentialsInEnvironment checks if AWS credentials are available in environment variables.
// This checks for AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, which are the standard AWS SDK env vars.
func HasAWSCredentialsInEnvironment() bool {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	return accessKeyID != "" && secretAccessKey != ""
}

// HasAWSWorkloadIdentityInEnvironment returns true if IRSA (EKS web identity) env vars are present.
func HasAWSWorkloadIdentityInEnvironment() bool {
	return os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" && os.Getenv("AWS_ROLE_ARN") != ""
}

// HasAWSContainerCredentialsInEnvironment returns true if ECS/EKS Pod Identity container credential env vars are present.
func HasAWSContainerCredentialsInEnvironment() bool {
	return os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != ""
}

// IsRunningOnAWS returns true if the code is likely running on AWS infrastructure.
// Checks static credentials, IRSA, container credentials, and IMDS in order.
func IsRunningOnAWS(ctx context.Context) bool {
	// Static credentials in environment
	if HasAWSCredentialsInEnvironment() {
		return true
	}
	// IRSA / EKS web identity
	if HasAWSWorkloadIdentityInEnvironment() {
		return true
	}
	// ECS task role or EKS Pod Identity container credentials
	if HasAWSContainerCredentialsInEnvironment() {
		return true
	}

	// Try to fetch instance identity document using ImdsAllVersions
	// This will try IMDSv2 first, then fallback to IMDSv1
	_, err := ec2internal.GetInstanceIdentity(ctx)
	return err == nil
}

// GetAWSRegion returns the AWS region for the current EC2 instance or from environment.
// Returns empty string and error if not running on AWS or if region cannot be determined.
// This function tries multiple methods in order:
// 1. AWS_REGION or AWS_DEFAULT_REGION environment variables
// 2. IMDS instance identity document (tries IMDSv2 first, then IMDSv1)
func GetAWSRegion(ctx context.Context) (string, error) {
	// First check environment variables (standard AWS SDK behavior)
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region, nil
	}
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		return region, nil
	}

	// Try to get region from IMDS (uses ImdsAllVersions to try v2, then v1)
	identity, err := ec2internal.GetInstanceIdentity(ctx)
	if err != nil {
		return "", err
	}

	return identity.Region, nil
}
