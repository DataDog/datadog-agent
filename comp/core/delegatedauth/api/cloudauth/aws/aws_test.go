// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

func TestGenerateAwsAuthData(t *testing.T) {
	auth := &AWSAuth{
		region: "us-east-1",
	}

	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		Token:           "test-session-token",
	}

	orgUUID := "test-org-uuid-12345"

	signingData, err := auth.generateAwsAuthData(context.Background(), orgUUID, awsCreds)
	require.NoError(t, err)
	require.NotNil(t, signingData)

	// Verify the SigningData structure is populated
	assert.NotEmpty(t, signingData.headersEncoded)
	assert.NotEmpty(t, signingData.bodyEncoded)
	assert.NotEmpty(t, signingData.urlEncoded)
	assert.Equal(t, "POST", signingData.method)

	// Decode and verify headers contain required fields
	headersJSON, err := base64.StdEncoding.DecodeString(signingData.headersEncoded)
	require.NoError(t, err)

	var headers map[string][]string
	err = json.Unmarshal(headersJSON, &headers)
	require.NoError(t, err)

	// Verify orgIDHeader is present and properly set
	// Note: HTTP headers are canonicalized, so x-ddog-org-id becomes X-Ddog-Org-Id
	assert.Contains(t, headers, "X-Ddog-Org-Id")
	assert.Equal(t, []string{orgUUID}, headers["X-Ddog-Org-Id"])

	// Verify Authorization header is present (signed by AWS SDK)
	assert.Contains(t, headers, "Authorization")
	assert.NotEmpty(t, headers["Authorization"])
	// Authorization header should contain AWS4-HMAC-SHA256
	assert.Contains(t, headers["Authorization"][0], "AWS4-HMAC-SHA256")
	// Verify that x-ddog-org-id is in the SignedHeaders list
	assert.Contains(t, headers["Authorization"][0], "SignedHeaders=")
	assert.Contains(t, headers["Authorization"][0], "x-ddog-org-id")

	// Verify session token header is present
	assert.Contains(t, headers, "X-Amz-Security-Token")
	assert.Equal(t, []string{awsCreds.Token}, headers["X-Amz-Security-Token"])

	// Verify other required headers
	assert.Contains(t, headers, "Content-Type")
	assert.Contains(t, headers, "User-Agent")
	assert.Contains(t, headers, "X-Amz-Date")

	// Decode and verify body
	bodyBytes, err := base64.StdEncoding.DecodeString(signingData.bodyEncoded)
	require.NoError(t, err)
	assert.Equal(t, getCallerIdentityBody, string(bodyBytes))

	// Decode and verify URL
	// Note: When region is specified (even as us-east-1), it uses regional endpoint
	urlBytes, err := base64.StdEncoding.DecodeString(signingData.urlEncoded)
	require.NoError(t, err)
	assert.Equal(t, "https://sts.us-east-1.amazonaws.com", string(urlBytes))
}

func TestGenerateAwsAuthDataWithDefaultEndpoint(t *testing.T) {
	auth := &AWSAuth{
		region: "", // Empty region should use global endpoint
	}

	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		Token:           "test-session-token",
	}

	orgUUID := "test-org-uuid-12345"

	signingData, err := auth.generateAwsAuthData(context.Background(), orgUUID, awsCreds)
	require.NoError(t, err)
	require.NotNil(t, signingData)

	// Decode and verify URL uses global endpoint
	urlBytes, err := base64.StdEncoding.DecodeString(signingData.urlEncoded)
	require.NoError(t, err)
	assert.Equal(t, "https://sts.amazonaws.com", string(urlBytes))
}

func TestGenerateAwsAuthDataWithRegionalEndpoint(t *testing.T) {
	auth := &AWSAuth{
		region: "eu-west-1",
	}

	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		Token:           "test-session-token",
	}

	orgUUID := "test-org-uuid-12345"

	signingData, err := auth.generateAwsAuthData(context.Background(), orgUUID, awsCreds)
	require.NoError(t, err)
	require.NotNil(t, signingData)

	// Decode and verify URL uses regional endpoint
	urlBytes, err := base64.StdEncoding.DecodeString(signingData.urlEncoded)
	require.NoError(t, err)
	assert.Equal(t, "https://sts.eu-west-1.amazonaws.com", string(urlBytes))
}

func TestGenerateAwsAuthDataMissingOrgUUID(t *testing.T) {
	auth := &AWSAuth{
		region: "us-east-1",
	}

	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		Token:           "test-session-token",
	}

	_, err := auth.generateAwsAuthData(context.Background(), "", awsCreds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing org UUID")
}

func TestGenerateAwsAuthDataMissingCredentials(t *testing.T) {
	auth := &AWSAuth{
		region: "us-east-1",
	}

	orgUUID := "test-org-uuid-12345"

	// Test with nil credentials
	_, err := auth.generateAwsAuthData(context.Background(), orgUUID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing AWS credentials")

	// Test with empty credentials
	_, err = auth.generateAwsAuthData(context.Background(), orgUUID, &creds.SecurityCredentials{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing AWS credentials")

	// Test with missing SecretAccessKey
	_, err = auth.generateAwsAuthData(context.Background(), orgUUID, &creds.SecurityCredentials{
		AccessKeyID: "test-access-key",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing AWS credentials")

	// Test with missing AccessKeyID
	_, err = auth.generateAwsAuthData(context.Background(), orgUUID, &creds.SecurityCredentials{
		SecretAccessKey: "test-secret-key",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing AWS credentials")
}

func TestGenerateAwsAuthDataWithoutToken(t *testing.T) {
	// Test that credentials without a Token (permanent IAM users) work correctly
	auth := &AWSAuth{
		region: "us-east-1",
	}

	// Permanent IAM user credentials (no session token)
	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Token:           "", // No session token for permanent IAM users
	}

	orgUUID := "test-org-uuid-12345"

	signingData, err := auth.generateAwsAuthData(context.Background(), orgUUID, awsCreds)
	require.NoError(t, err)
	require.NotNil(t, signingData)

	// Verify the SigningData structure is populated
	assert.NotEmpty(t, signingData.headersEncoded)
	assert.NotEmpty(t, signingData.bodyEncoded)
	assert.NotEmpty(t, signingData.urlEncoded)
	assert.Equal(t, "POST", signingData.method)

	// Decode and verify headers
	headersJSON, err := base64.StdEncoding.DecodeString(signingData.headersEncoded)
	require.NoError(t, err)

	var headers map[string][]string
	err = json.Unmarshal(headersJSON, &headers)
	require.NoError(t, err)

	// Verify Authorization header is present and properly signed
	assert.Contains(t, headers, "Authorization")
	assert.Contains(t, headers["Authorization"][0], "AWS4-HMAC-SHA256")

	// X-Amz-Security-Token should NOT be present for permanent credentials
	assert.NotContains(t, headers, "X-Amz-Security-Token")
}
