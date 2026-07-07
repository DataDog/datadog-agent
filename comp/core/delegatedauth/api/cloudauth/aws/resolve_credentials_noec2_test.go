// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ec2

package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestResolveCredentials_NoCredsReturnsEmpty verifies the non-ec2 (env-only) resolver returns empty
// credentials when no AWS credential env vars are set. The ec2 build covers the no-credentials path
// via provider selection (TestCredentialProvider_EC2_Selection) and the provider-error tests,
// without reaching live IMDS.
func TestResolveCredentials_NoCredsReturnsEmpty(t *testing.T) {
	isolateAWSEnv(t)

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NotNil(t, got)
	// AccessKeyID will be empty; downstream generateAwsAuthData returns "missing AWS credentials".
	assert.Empty(t, got.AccessKeyID)
}
