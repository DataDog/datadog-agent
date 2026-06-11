// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"testing"

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
