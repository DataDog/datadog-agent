// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

// isolateAWSEnv makes credential resolution hermetic: it clears every AWS credential-source
// environment variable, neutralizes AWS_PROFILE, and points the shared config/credentials files at
// a nonexistent path so tests do not pick up the developer or CI machine's AWS configuration.
// credentialProvider selects a provider from these env vars, so a stray AWS_ROLE_ARN or container
// URI on the host would otherwise change which provider is chosen. Tests deliberately
// avoid driving the default IMDS branch through resolveCredentials (which would make a live
// metadata call, ignoring AWS_EC2_METADATA_DISABLED since the Agent governs IMDS via its own
// config); they assert provider selection or inject the fetch instead.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
		"AWS_REGION", "AWS_DEFAULT_REGION",
	} {
		t.Setenv(k, "")
	}
	missing := filepath.Join(t.TempDir(), "no-such-aws-file")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
}

// -- Static env var tests --

func TestResolveCredentials_StaticEnvVars(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret123")
	t.Setenv("AWS_SESSION_TOKEN", "token456")

	auth := &AWSAuth{region: "us-east-1"}
	got, err := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NoError(t, err)
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
	got, err := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secret123", got.SecretAccessKey)
	assert.Empty(t, got.Token)
}

// TestResolveCredentials_StaticEnvVarsReturned verifies the static-env provider is selected
// and returns the credentials.
func TestResolveCredentials_StaticEnvVarsReturned(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "EKSTATICKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "EKSTATICSECRET")
	t.Setenv("AWS_SESSION_TOKEN", "EKSTATICTOKEN")

	auth := &AWSAuth{region: "eu-west-1"}
	got, err := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, creds.SourceEnvironment, auth.LastCredentialSource())
	assert.Equal(t, "EKSTATICKEY", got.AccessKeyID)
	assert.Equal(t, "EKSTATICSECRET", got.SecretAccessKey)
	assert.Equal(t, "EKSTATICTOKEN", got.Token)
}

// TestResolveCredentials_ProviderFailureIsAttributed verifies a failing provider yields an error
// naming the credential mechanism that was tried, and that the mechanism is recorded for the status
// page even though the attempt failed. It forces a deterministic web-identity failure via a missing
// token file rather than falling through to the IMDS provider, which would make a live metadata
// call on an EC2 host or CI runner.
func TestResolveCredentials_ProviderFailureIsAttributed(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/example")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", filepath.Join(t.TempDir(), "no-such-token"))

	auth := &AWSAuth{region: "us-east-1"}
	got, err := auth.resolveCredentials(context.Background(), configmock.New(t))
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), creds.SourceWebIdentity)
	assert.Equal(t, creds.SourceWebIdentity, auth.LastCredentialSource())
}

// TestResolveCredentials_EmptyCredentialsAreAnError verifies a provider that succeeds but hands
// back blank credentials is treated as a failure. The Agent's IMDS helper unmarshals whatever JSON
// the metadata endpoint returns, so an error document served with a 200 produces exactly this
// shape; without the check the caller would log a successful resolution and then fail to sign.
func TestResolveCredentials_EmptyCredentialsAreAnError(t *testing.T) {
	isolateAWSEnv(t)

	auth := &AWSAuth{region: "us-east-1"}
	// Stand in for the IMDS leg, whose error document yields such empty credentials.
	provider := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, nil
	})
	sdkCreds, err := provider.Retrieve(context.Background())
	require.NoError(t, err)
	require.Empty(t, sdkCreds.AccessKeyID)

	// resolveCredentials rejects that result rather than passing it on.
	got, err := auth.resolveCredentialsFrom(context.Background(), provider, creds.SourceIMDS)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "empty credentials")
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

// TestResolveCredentialsFrom_ContainerErrorDocument checks that resolveCredentialsFrom rejects the
// zero-valued credentials a container endpoint error document yields.
func TestResolveCredentialsFrom_ContainerErrorDocument(t *testing.T) {
	// A 200 carrying an error document unmarshals cleanly into zero values; resolveCredentialsFrom
	// is what must catch that, so confirm the pair behaves rather than reporting a false success.
	t.Run("blank credentials are rejected by the caller", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"Code":"InternalError"}`))
		}))
		defer srv.Close()

		isolateAWSEnv(t)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", srv.URL)
		p, _, err := creds.CredentialProvider(configmock.New(t), "")
		require.NoError(t, err)
		auth := &AWSAuth{}
		_, err = auth.resolveCredentialsFrom(context.Background(), p, creds.SourceContainer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "returned empty credentials")
	})
}
