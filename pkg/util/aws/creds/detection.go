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

// IsRunningOnAWS returns true if the code is running on an AWS EC2 instance.
// This attempts to detect AWS using both IMDSv2 (preferred) and IMDSv1 (fallback).
// It also checks for AWS credentials in environment variables as an additional signal.
func IsRunningOnAWS(ctx context.Context) bool {
	// First, check if AWS credentials are explicitly set in environment
	// This is a strong signal that the user intends to use AWS
	if HasAWSCredentialsInEnvironment() {
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
