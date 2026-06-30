// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveCredentials_EC2_StaticEnvVarsReturned verifies the static-env provider is selected
// and returns the credentials.
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

// TestResolveRegion_EC2 covers the region precedence used for the IRSA STS call. The IRSA-only
// case (no configured region, no AWS_REGION/AWS_DEFAULT_REGION) must still yield a region,
// otherwise the web-identity STS call fails endpoint resolution.
func TestResolveRegion_EC2(t *testing.T) {
	t.Run("configured region wins", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_REGION", "ap-southeast-2")
		assert.Equal(t, "eu-west-1", (&AWSAuth{region: "eu-west-1"}).resolveRegion())
	})
	t.Run("AWS_REGION when unconfigured", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_REGION", "ap-southeast-2")
		assert.Equal(t, "ap-southeast-2", (&AWSAuth{}).resolveRegion())
	})
	t.Run("AWS_DEFAULT_REGION fallback", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_DEFAULT_REGION", "us-west-2")
		assert.Equal(t, "us-west-2", (&AWSAuth{}).resolveRegion())
	})
	t.Run("defaultRegion when nothing set (IRSA-only pod)", func(t *testing.T) {
		isolateAWSEnv(t)
		assert.Equal(t, defaultRegion, (&AWSAuth{}).resolveRegion())
	})
}

// TestCredentialProvider_EC2_Selection verifies the env-driven provider selection follows the
// SDK precedence: static env -> IRSA web identity -> container -> IMDS.
func TestCredentialProvider_EC2_Selection(t *testing.T) {
	t.Run("static env", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_ACCESS_KEY_ID", "k")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "s")
		p, err := (&AWSAuth{}).credentialProvider()
		require.NoError(t, err)
		assert.IsType(t, credentials.StaticCredentialsProvider{}, p)
	})
	t.Run("IRSA web identity", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
		p, err := (&AWSAuth{}).credentialProvider()
		require.NoError(t, err)
		assert.IsType(t, &stscreds.WebIdentityRoleProvider{}, p)
	})
	t.Run("container credentials", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc")
		p, err := (&AWSAuth{}).credentialProvider()
		require.NoError(t, err)
		assert.IsType(t, &endpointcreds.Provider{}, p)
	})
	t.Run("IMDS default", func(t *testing.T) {
		isolateAWSEnv(t)
		p, err := (&AWSAuth{}).credentialProvider()
		require.NoError(t, err)
		assert.IsType(t, &ec2rolecreds.Provider{}, p)
	})
}

// TestContainerCredentialsProvider_HostAllowlist verifies the SSRF guard on an http
// AWS_CONTAINER_CREDENTIALS_FULL_URI: link-local ECS/EKS and loopback hosts are accepted, an
// arbitrary host is rejected, and https is trusted as-is.
func TestContainerCredentialsProvider_HostAllowlist(t *testing.T) {
	t.Run("EKS Pod Identity link-local accepted", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://169.254.170.23/v1/credentials")
		_, err := containerCredentialsProvider()
		assert.NoError(t, err)
	})
	t.Run("arbitrary http host rejected", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://169.254.169.254/latest/meta-data")
		_, err := containerCredentialsProvider()
		assert.Error(t, err)
	})
	t.Run("external https host trusted", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "https://creds.internal.example/v1")
		_, err := containerCredentialsProvider()
		assert.NoError(t, err)
	})
}
