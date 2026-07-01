// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
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
		assert.IsType(t, &webIdentityProvider{}, p)
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

// TestWebIdentityProvider_Retrieve verifies the hand-rolled AssumeRoleWithWebIdentity call: it
// POSTs the query-API form with the token from the file and parses credentials from the STS XML.
func TestWebIdentityProvider_Retrieve(t *testing.T) {
	const respXML = `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE</AccessKeyId>
      <SecretAccessKey>secretexample</SecretAccessKey>
      <SessionToken>tokenexample</SessionToken>
      <Expiration>2030-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`

	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, respXML)
	}))
	defer srv.Close()

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("the-web-identity-jwt"), 0o600))

	p := &webIdentityProvider{
		roleARN:   "arn:aws:iam::123456789012:role/example",
		tokenFile: tokenFile,
		stsURL:    srv.URL,
		client:    srv.Client(),
	}
	got, err := p.Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ASIAEXAMPLE", got.AccessKeyID)
	assert.Equal(t, "secretexample", got.SecretAccessKey)
	assert.Equal(t, "tokenexample", got.SessionToken)
	assert.True(t, got.CanExpire)

	// The request carried the right STS action, role, and the token read from the file.
	assert.Equal(t, "AssumeRoleWithWebIdentity", gotForm.Get("Action"))
	assert.Equal(t, "arn:aws:iam::123456789012:role/example", gotForm.Get("RoleArn"))
	assert.Equal(t, "the-web-identity-jwt", gotForm.Get("WebIdentityToken"))
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

// TestWebIdentityProvider_Retrieve_NoExpiration verifies that an STS response without an
// <Expiration> yields non-expiring credentials (CanExpire false), which the caller relies on.
func TestWebIdentityProvider_Retrieve_NoExpiration(t *testing.T) {
	const respXML = `<AssumeRoleWithWebIdentityResponse><AssumeRoleWithWebIdentityResult><Credentials>` +
		`<AccessKeyId>AKID</AccessKeyId><SecretAccessKey>SK</SecretAccessKey><SessionToken>TK</SessionToken>` +
		`</Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, respXML)
	}))
	defer srv.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("jwt"), 0o600))

	got, err := (&webIdentityProvider{roleARN: "r", tokenFile: tokenFile, stsURL: srv.URL, client: srv.Client()}).Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "AKID", got.AccessKeyID)
	assert.False(t, got.CanExpire)
}

// TestWebIdentityProvider_Retrieve_Errors verifies error paths: a non-200 STS response and a
// missing token file both return an error and no credentials.
func TestWebIdentityProvider_Retrieve_Errors(t *testing.T) {
	t.Run("STS non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<ErrorResponse><Error><Message>not authorized</Message></Error></ErrorResponse>`)
		}))
		defer srv.Close()
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("jwt"), 0o600))
		got, err := (&webIdentityProvider{roleARN: "r", tokenFile: tokenFile, stsURL: srv.URL, client: srv.Client()}).Retrieve(context.Background())
		require.Error(t, err)
		assert.Empty(t, got.AccessKeyID)
	})
	t.Run("missing token file", func(t *testing.T) {
		got, err := (&webIdentityProvider{roleARN: "r", tokenFile: "/no/such/token", stsURL: "https://sts.us-east-1.amazonaws.com/", client: http.DefaultClient}).Retrieve(context.Background())
		require.Error(t, err)
		assert.Empty(t, got.AccessKeyID)
	})
}
