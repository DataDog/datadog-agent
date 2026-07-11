// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// isolateAWSEnv makes credential resolution hermetic: it clears every AWS credential-source
// environment variable, neutralizes AWS_PROFILE, and points the shared config/credentials files at
// a nonexistent path so tests do not pick up the developer or CI machine's AWS configuration. In
// the ec2 build credentialProvider selects a provider from these env vars, so a stray AWS_ROLE_ARN
// or container URI on the host would otherwise change which provider is chosen. Tests deliberately
// avoid driving the default IMDS branch through resolveCredentials (which would make a live
// metadata call, ignoring AWS_EC2_METADATA_DISABLED since the Agent governs IMDS via its own
// config); they assert provider selection or inject the fetch instead.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_REGION", "AWS_DEFAULT_REGION",
	} {
		t.Setenv(k, "")
	}
	missing := filepath.Join(t.TempDir(), "no-such-aws-file")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
}

// -- Static env var tests (run in both build variants) --

func TestResolveCredentials_StaticEnvVars(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret123")
	t.Setenv("AWS_SESSION_TOKEN", "token456")

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NotNil(t, got)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secret123", got.SecretAccessKey)
	assert.Equal(t, "token456", got.Token)
}

func TestResolveCredentials_StaticEnvVars_NoToken(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret123")

	auth := &AWSAuth{region: "us-east-1"}
	got := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NotNil(t, got)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secret123", got.SecretAccessKey)
	assert.Empty(t, got.Token)
}
