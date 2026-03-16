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

func TestIsRunningOnAWSWithCredentials(t *testing.T) {
	// When AWS credentials are set in environment, IsRunningOnAWS should return true
	// even without IMDS access
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	result := IsRunningOnAWS(context.Background())
	assert.True(t, result)
}

func TestIsRunningOnAWSWithIMDS(t *testing.T) {
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

	result := IsRunningOnAWS(context.Background())
	assert.True(t, result)
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
