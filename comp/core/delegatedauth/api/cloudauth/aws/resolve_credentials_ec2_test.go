// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveCredentials_EC2_NoSourceReturnsEmptySecurityCredentials verifies that when no
// credential source is available (env vars cleared, shared config isolated, IMDS disabled),
// the ec2 build returns a non-nil *SecurityCredentials with empty fields -- not nil and not
// an error. This pins the error-to-empty contract so callers can always read AccessKeyID safely.
func TestResolveCredentials_EC2_NoSourceReturnsEmptySecurityCredentials(t *testing.T) {
	isolateAWSEnv(t)

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background())
	require.NotNil(t, got, "resolveCredentials must never return nil in ec2 build")
	// AccessKeyID will be empty when no source is available.
	assert.Empty(t, got.AccessKeyID)
}

// TestResolveCredentials_EC2_StaticEnvVarsReturned verifies that static env credentials are
// returned by the SDK chain.
func TestResolveCredentials_EC2_StaticEnvVarsReturned(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "EKSTATICKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "EKSTATICSECRET")
	t.Setenv("AWS_SESSION_TOKEN", "EKSTATICTOKEN")

	auth := &AWSAuth{region: "eu-west-1"}
	got := auth.resolveCredentials(context.Background())
	require.NotNil(t, got)
	assert.Equal(t, "EKSTATICKEY", got.AccessKeyID)
	assert.Equal(t, "EKSTATICSECRET", got.SecretAccessKey)
	assert.Equal(t, "EKSTATICTOKEN", got.Token)
}

// resolvedRegion applies the region load options the way resolveCredentials does and returns
// the region the SDK settled on, so we can assert region resolution without an STS call.
func resolvedRegion(t *testing.T, auth *AWSAuth) string {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), auth.regionLoadOptions()...)
	require.NoError(t, err)
	return cfg.Region
}

// TestResolveCredentials_EC2_RegionFallback covers region resolution precedence. The
// IRSA-only case (no configured region, no AWS_REGION/AWS_DEFAULT_REGION) must still yield a
// region, otherwise the SDK web-identity provider's STS call fails endpoint resolution.
func TestResolveCredentials_EC2_RegionFallback(t *testing.T) {
	t.Run("IRSA-only pod with no region falls back to defaultRegion", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")

		assert.Equal(t, defaultRegion, resolvedRegion(t, &AWSAuth{}))
	})

	t.Run("env region is honored when no region is configured", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_REGION", "ap-southeast-2")

		assert.Equal(t, "ap-southeast-2", resolvedRegion(t, &AWSAuth{}))
	})

	t.Run("configured region overrides env region", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_REGION", "ap-southeast-2")

		assert.Equal(t, "eu-west-1", resolvedRegion(t, &AWSAuth{region: "eu-west-1"}))
	})
}
