// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package creds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

func TestHasAWSCredentialsInEnvironment(t *testing.T) {
	tests := []struct {
		name            string
		accessKeyID     string
		secretAccessKey string
		expected        bool
	}{
		{
			name:            "both credentials set",
			accessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			expected:        true,
		},
		{
			name:            "only access key set",
			accessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			secretAccessKey: "",
			expected:        false,
		},
		{
			name:            "only secret key set",
			accessKeyID:     "",
			secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			expected:        false,
		},
		{
			name:            "neither credential set",
			accessKeyID:     "",
			secretAccessKey: "",
			expected:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			if tc.accessKeyID != "" {
				t.Setenv("AWS_ACCESS_KEY_ID", tc.accessKeyID)
			}
			if tc.secretAccessKey != "" {
				t.Setenv("AWS_SECRET_ACCESS_KEY", tc.secretAccessKey)
			}

			result := HasAWSCredentialsInEnvironment()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetAWSRegionFromEnvironment(t *testing.T) {
	tests := []struct {
		name           string
		awsRegion      string
		awsDefaultReg  string
		expectedRegion string
		expectError    bool
	}{
		{
			name:           "AWS_REGION set",
			awsRegion:      "us-west-2",
			awsDefaultReg:  "",
			expectedRegion: "us-west-2",
			expectError:    false,
		},
		{
			name:           "AWS_DEFAULT_REGION set",
			awsRegion:      "",
			awsDefaultReg:  "eu-west-1",
			expectedRegion: "eu-west-1",
			expectError:    false,
		},
		{
			name:           "AWS_REGION takes precedence",
			awsRegion:      "us-east-1",
			awsDefaultReg:  "eu-west-1",
			expectedRegion: "us-east-1",
			expectError:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Explicitly set both env vars to ensure isolation from any pre-existing values
			// t.Setenv will restore the original value (or unset) after the test
			t.Setenv("AWS_REGION", tc.awsRegion)
			t.Setenv("AWS_DEFAULT_REGION", tc.awsDefaultReg)

			region, err := GetAWSRegion(context.Background())

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedRegion, region)
			}
		})
	}
}

// isolateAWSCredentialEnv clears every AWS credential-source environment variable so a test
// asserting one specific source wins is not at the mercy of the host's ambient environment (a
// developer machine or CI runner with AWS_ACCESS_KEY_ID set would otherwise make static env win
// over whatever this test means to exercise).
func isolateAWSCredentialEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
	} {
		t.Setenv(k, "")
	}
}

func TestDetectAWSCredentialSourceWithCredentials(t *testing.T) {
	// When AWS credentials are set in environment, detection should succeed even without IMDS access
	isolateAWSCredentialEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	source, err := DetectAWSCredentialSource(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, SourceEnvironment, source)
}

func TestDetectAWSCredentialSourceWithIMDS(t *testing.T) {
	// A stray credential env var on the host running this test would otherwise win over IMDS.
	isolateAWSCredentialEnv(t)

	// Create a mock IMDS server
	identityDoc := ec2internal.EC2Identity{
		Region:     "us-west-2",
		InstanceID: "i-1234567890abcdef0",
		AccountID:  "123456789012",
	}
	identityJSON, err := json.Marshal(identityDoc)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle token request for IMDSv2
		if r.URL.Path == "/latest/api/token" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("mock-token"))
			return
		}
		// Handle instance identity request
		if r.URL.Path == "/latest/dynamic/instance-identity/document/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(identityJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Override the internal URLs to point to our mock server
	originalTokenURL := ec2internal.TokenURL
	originalIdentityURL := ec2internal.InstanceIdentityURL
	ec2internal.TokenURL = server.URL + "/latest/api/token"
	ec2internal.InstanceIdentityURL = server.URL + "/latest/dynamic/instance-identity/document/"
	defer func() {
		ec2internal.TokenURL = originalTokenURL
		ec2internal.InstanceIdentityURL = originalIdentityURL
	}()

	source, err := DetectAWSCredentialSource(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, SourceIMDS, source)
}

func TestGetAWSRegionFromIMDS(t *testing.T) {
	// Clear environment variables to ensure IMDS is used
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	// Create a mock IMDS server
	identityDoc := ec2internal.EC2Identity{
		Region:     "ap-northeast-1",
		InstanceID: "i-1234567890abcdef0",
		AccountID:  "123456789012",
	}
	identityJSON, err := json.Marshal(identityDoc)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle token request for IMDSv2
		if r.URL.Path == "/latest/api/token" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("mock-token"))
			return
		}
		// Handle instance identity request
		if r.URL.Path == "/latest/dynamic/instance-identity/document/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(identityJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Override the internal URLs to point to our mock server
	originalTokenURL := ec2internal.TokenURL
	originalIdentityURL := ec2internal.InstanceIdentityURL
	ec2internal.TokenURL = server.URL + "/latest/api/token"
	ec2internal.InstanceIdentityURL = server.URL + "/latest/dynamic/instance-identity/document/"
	defer func() {
		ec2internal.TokenURL = originalTokenURL
		ec2internal.InstanceIdentityURL = originalIdentityURL
	}()

	region, err := GetAWSRegion(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "ap-northeast-1", region)
}

func TestDetectAWSCredentialSourceWithIRSAEnvVars(t *testing.T) {
	// IRSA env vars should signal AWS even without IMDS
	isolateAWSCredentialEnv(t)
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/test-role")

	source, err := DetectAWSCredentialSource(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, SourceWebIdentity, source)
}

func TestDetectAWSCredentialSourceWithContainerRelativeURI(t *testing.T) {
	// ECS task role / EKS Pod Identity relative URI
	isolateAWSCredentialEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc123def456")

	source, err := DetectAWSCredentialSource(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, SourceContainer, source)
}

func TestDetectAWSCredentialSourceWithContainerFullURI(t *testing.T) {
	// EKS Pod Identity full URI variant
	isolateAWSCredentialEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://169.254.170.23/v1/credentials")

	source, err := DetectAWSCredentialSource(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, SourceContainer, source)
}

func TestHasAWSWorkloadIdentityInEnvironment(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/token")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/r")
	assert.True(t, HasAWSWorkloadIdentityInEnvironment())
}

func TestHasAWSWorkloadIdentityInEnvironment_OnlyToken(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/token")
	assert.False(t, HasAWSWorkloadIdentityInEnvironment())
}

func TestHasAWSContainerCredentialsInEnvironment(t *testing.T) {
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/creds")
	assert.True(t, HasAWSContainerCredentialsInEnvironment())
}

// TestDetectAWSCredentialSourceNoSource covers the case that made this feature hard to support: no
// credential source at all. The returned error is what the Agent logs and shows in `agent status`,
// so it must name every mechanism that was checked. IMDS is pointed at a closed port so the test
// does not depend on whether the machine running it happens to be an EC2 instance.
func TestDetectAWSCredentialSourceNoSource(t *testing.T) {
	isolateAWSCredentialEnv(t)

	// Point IMDS at a listener that accepts nothing, so detection fails deterministically.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	originalTokenURL := ec2internal.TokenURL
	originalIdentityURL := ec2internal.InstanceIdentityURL
	ec2internal.TokenURL = server.URL + "/latest/api/token"
	ec2internal.InstanceIdentityURL = server.URL + "/latest/dynamic/instance-identity/document/"
	defer func() {
		ec2internal.TokenURL = originalTokenURL
		ec2internal.InstanceIdentityURL = originalIdentityURL
	}()

	source, err := DetectAWSCredentialSource(context.Background())
	require.Error(t, err)
	assert.Empty(t, source)
	// The message must point the operator at each mechanism, not just say "not on AWS".
	assert.Contains(t, err.Error(), "AWS_ACCESS_KEY_ID")
	assert.Contains(t, err.Error(), "AWS_WEB_IDENTITY_TOKEN_FILE")
	assert.Contains(t, err.Error(), "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	assert.Contains(t, err.Error(), "IMDS")
}

func TestIncompleteAWSCredentialEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "nothing set", env: nil, want: ""},
		{
			name: "complete static pair is not incomplete",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKID", "AWS_SECRET_ACCESS_KEY": "SECRET"},
			want: "",
		},
		{
			name: "access key without secret",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKID"},
			want: "AWS_ACCESS_KEY_ID is set but AWS_SECRET_ACCESS_KEY is not",
		},
		{
			name: "secret without access key",
			env:  map[string]string{"AWS_SECRET_ACCESS_KEY": "SECRET"},
			want: "AWS_SECRET_ACCESS_KEY is set but AWS_ACCESS_KEY_ID is not",
		},
		{
			name: "complete IRSA pair is not incomplete",
			env:  map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::1:role/r", "AWS_WEB_IDENTITY_TOKEN_FILE": "/t"},
			want: "",
		},
		{
			name: "role arn without token file",
			env:  map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::1:role/r"},
			want: "AWS_ROLE_ARN is set but AWS_WEB_IDENTITY_TOKEN_FILE is not",
		},
		{
			name: "token file without role arn",
			env:  map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/t"},
			want: "AWS_WEB_IDENTITY_TOKEN_FILE is set but AWS_ROLE_ARN is not",
		},
		{
			// Either container variable alone is a complete configuration, so neither counts.
			name: "container relative uri alone is complete",
			env:  map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/creds"},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateAWSCredentialEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.want, IncompleteAWSCredentialEnv())
		})
	}
}

// An operator who excludes aws from cloud_provider_metadata has asked the Agent not to contact
// IMDS. Detection's IMDS fallback reaches DoHTTPRequest directly, so without an explicit check it
// would probe the endpoint anyway.
func TestDetectAWSCredentialSourceHonorsCloudProviderMetadata(t *testing.T) {
	isolateAWSCredentialEnv(t)
	configmock.NewFromYAML(t, "cloud_provider_metadata:\n  - gcp\n  - azure\n")

	source, err := DetectAWSCredentialSource(context.Background())
	require.Error(t, err)
	assert.Empty(t, source)
	assert.ErrorIs(t, err, ec2internal.ErrCloudProviderDisabled)
}

// The env-based sources are pure environment inspection and make no request, so the opt-out must
// not suppress them.
func TestDetectAWSCredentialSourceEnvSourcesIgnoreCloudProviderMetadata(t *testing.T) {
	isolateAWSCredentialEnv(t)
	configmock.NewFromYAML(t, "cloud_provider_metadata:\n  - gcp\n")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")

	source, err := DetectAWSCredentialSource(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SourceEnvironment, source)
}

// An IRSA or container-credential workload reaches GetAWSRegion with no AWS_REGION set, so this
// IMDS lookup is the one metadata request the other two guards do not cover.
func TestGetAWSRegionHonorsCloudProviderMetadata(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	configmock.NewFromYAML(t, "cloud_provider_metadata:\n  - gcp\n  - azure\n")

	region, err := GetAWSRegion(context.Background())
	require.Error(t, err)
	assert.Empty(t, region)
	assert.ErrorIs(t, err, ec2internal.ErrCloudProviderDisabled)
}

// The env vars are read before the guard, so an explicitly configured region still resolves with
// AWS excluded from cloud_provider_metadata.
func TestGetAWSRegionEnvIgnoresCloudProviderMetadata(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	configmock.NewFromYAML(t, "cloud_provider_metadata:\n  - gcp\n")

	region, err := GetAWSRegion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", region)
}
