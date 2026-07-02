// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package creds

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

func TestGetIAMRole(t *testing.T) {
	ctx := context.Background()
	const expected = "test-role"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, expected)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL

	val, err := getIAMRole(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
}

func TestGetSecurityCredentials(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "test-role")
		} else if r.URL.Path == "/iam/security-credentials/test-role" {
			w.Header().Set("Content-Type", "text/plain")
			content, err := os.ReadFile("tags/payloads/security_cred.json")
			require.NoError(t, err, fmt.Sprintf("failed to load json in tags/payloads/security_cred.json: %v", err))
			w.Write(content)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL

	cred, err := GetSecurityCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "123456", cred.AccessKeyID)
	assert.Equal(t, "secret access key", cred.SecretAccessKey)
	assert.Equal(t, "secret token", cred.Token)
}

func TestGetECSSecurityCredentials(t *testing.T) {
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"AccessKeyId":     "AKIAIOSFODNN7EXAMPLE",
			"SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"Token":           "test-session-token",
			"Expiration":      "2099-01-01T00:00:00Z"
		}`))
	}))
	defer ts.Close()

	t.Run("via AWS_CONTAINER_CREDENTIALS_FULL_URI", func(t *testing.T) {
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ts.URL)
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

		cred, err := GetECSSecurityCredentials(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cred.AccessKeyID)
		assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", cred.SecretAccessKey)
		assert.Equal(t, "test-session-token", cred.Token)
	})

	t.Run("via AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", func(t *testing.T) {
		// Extract host from test server URL and use it as base; relative URI is just "/"
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

		// For relative URI we need to mock out the base URL — use full URI instead
		// since the base (169.254.170.2) is not reachable in unit tests.
		// This is covered by the FULL_URI test above; relative URI path is tested via
		// an env-var-only check in detection_test.go (IsRunningOnECS).
		t.Skip("relative URI requires 169.254.170.2 base — covered by full URI test")
	})

	t.Run("no ECS env vars returns error", func(t *testing.T) {
		t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
		t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

		_, err := GetECSSecurityCredentials(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AWS_CONTAINER_CREDENTIALS")
	})
}

func TestGetECSSecurityCredentialsWithAuthToken(t *testing.T) {
	ctx := context.Background()
	const expectedToken = "my-auth-token"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"AccessKeyId":     "AKIAIOSFODNN7EXAMPLE",
			"SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"Token":           "test-session-token"
		}`))
	}))
	defer ts.Close()

	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ts.URL)

	t.Run("plain token from AWS_CONTAINER_AUTHORIZATION_TOKEN", func(t *testing.T) {
		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", expectedToken)
		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", "")

		cred, err := GetECSSecurityCredentials(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cred.AccessKeyID)
	})

	t.Run("token from AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE (EKS Pod Identity)", func(t *testing.T) {
		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "")

		// Write token to a temp file
		f, err := os.CreateTemp("", "ecs-token-*")
		require.NoError(t, err)
		defer os.Remove(f.Name())
		f.WriteString(expectedToken + "\n") // include trailing newline to test TrimSpace
		f.Close()

		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", f.Name())

		cred, err := GetECSSecurityCredentials(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cred.AccessKeyID)
	})

	t.Run("TOKEN_FILE takes precedence over TOKEN", func(t *testing.T) {
		// File has the correct token; plain env var has wrong one
		f, err := os.CreateTemp("", "ecs-token-*")
		require.NoError(t, err)
		defer os.Remove(f.Name())
		f.WriteString(expectedToken)
		f.Close()

		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", f.Name())
		t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "wrong-token")

		cred, err := GetECSSecurityCredentials(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cred.AccessKeyID)
	})
}
