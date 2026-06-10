// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Static env var tests (run in both build variants) --

func TestResolveCredentials_StaticEnvVars(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret123")
	t.Setenv("AWS_SESSION_TOKEN", "token456")

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background())
	require.NotNil(t, got)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secret123", got.SecretAccessKey)
	assert.Equal(t, "token456", got.Token)
}

func TestResolveCredentials_StaticEnvVars_NoToken(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret123")

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background())
	require.NotNil(t, got)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secret123", got.SecretAccessKey)
	assert.Empty(t, got.Token)
}

func TestResolveCredentials_NoCredsReturnsEmpty(t *testing.T) {
	// Clear any ambient AWS credential source so the test is hermetic on developer
	// or CI machines that already have credentials configured.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_ROLE_ARN", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	// Disable IMDS so the ec2-build SDK chain cannot reach instance metadata on EC2 CI runners.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background())
	require.NotNil(t, got)
	// AccessKeyID will be empty; downstream generateAwsAuthData returns "missing AWS credentials"
	assert.Empty(t, got.AccessKeyID)
}
