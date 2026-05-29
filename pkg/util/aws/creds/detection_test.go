// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package creds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestIsRunningOnECS(t *testing.T) {
	tests := []struct {
		name         string
		relativeURI  string
		fullURI      string
		expected     bool
	}{
		{
			name:        "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI set",
			relativeURI: "/v2/credentials/abc123",
			expected:    true,
		},
		{
			name:     "AWS_CONTAINER_CREDENTIALS_FULL_URI set",
			fullURI:  "http://169.254.170.2/v2/credentials/abc123",
			expected: true,
		},
		{
			name:     "neither ECS env var set",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", tc.relativeURI)
			t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", tc.fullURI)
			assert.Equal(t, tc.expected, IsRunningOnECS())
		})
	}
}

func TestIsRunningOnAWSWithECSCredentials(t *testing.T) {
	// IsRunningOnAWS should return true when ECS container credential env vars are set,
	// even without IMDS access.
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc123")
	result := IsRunningOnAWS(context.Background())
	assert.True(t, result)
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

func TestIsRunningOnAWSWithCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	result := IsRunningOnAWS(context.Background())
	assert.True(t, result)
}

func TestIsRunningOnAWSWithIMDS(t *testing.T) {
	identityDoc := ec2internal.EC2Identity{
		Region:     "us-west-2",
		InstanceID: "i-1234567890abcdef0",
		AccountID:  "123456789012",
	}
	identityJSON, err := json.Marshal(identityDoc)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest/api/token" {
			w.Write([]byte("mock-token"))
			return
		}
		if r.URL.Path == "/latest/dynamic/instance-identity/document/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(identityJSON)
			return
		}
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

	result := IsRunningOnAWS(context.Background())
	assert.True(t, result)
}

func TestGetAWSRegionFromIMDS(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	identityDoc := ec2internal.EC2Identity{
		Region:     "ap-northeast-1",
		InstanceID: "i-1234567890abcdef0",
		AccountID:  "123456789012",
	}
	identityJSON, err := json.Marshal(identityDoc)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest/api/token" {
			w.Write([]byte("mock-token"))
			return
		}
		if r.URL.Path == "/latest/dynamic/instance-identity/document/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(identityJSON)
			return
		}
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

	region, err := GetAWSRegion(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "ap-northeast-1", region)
}
