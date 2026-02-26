// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build integration

package aws

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

// TestGenerateAwsAuthDataIntegration is an integration test that uses real AWS credentials
// to verify that our signing implementation works with the actual AWS STS service.
//
// Run with: aws-vault exec sso-build-stable-developer -- go test -v -tags=integration ./comp/core/delegatedauth/api/cloudauth/... -run TestGenerateAwsAuthDataIntegration
func TestGenerateAwsAuthDataIntegration(t *testing.T) {
	// Check that AWS credentials are available
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKeyID == "" || secretAccessKey == "" {
		t.Skip("AWS credentials not available (AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY required)")
	}

	// Use a test org UUID
	orgUUID := "test-org-uuid-12345"

	// Create credentials
	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Token:           sessionToken,
	}

	// Test with both empty region (global) and specific region
	testCases := []struct {
		name   string
		region string
	}{
		{
			name:   "Global endpoint (us-east-1)",
			region: "",
		},
		{
			name:   "Regional endpoint (us-west-2)",
			region: "us-west-2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			auth := &AWSAuth{
				AwsRegion: tc.region,
			}

			// Generate the signing data
			signingData, err := auth.generateAwsAuthData(orgUUID, awsCreds)
			require.NoError(t, err)
			require.NotNil(t, signingData)

			t.Logf("Generated signing data for region: %s", tc.region)

			// Decode the components
			headersJSON, err := base64.StdEncoding.DecodeString(signingData.HeadersEncoded)
			require.NoError(t, err)

			var headers map[string][]string
			err = json.Unmarshal(headersJSON, &headers)
			require.NoError(t, err)

			bodyBytes, err := base64.StdEncoding.DecodeString(signingData.BodyEncoded)
			require.NoError(t, err)

			urlBytes, err := base64.StdEncoding.DecodeString(signingData.URLEncoded)
			require.NoError(t, err)
			stsURL := string(urlBytes)

			t.Logf("STS URL: %s", stsURL)
			t.Logf("Headers: %+v", headers)
			t.Logf("Body: %s", string(bodyBytes))

			// Make the actual HTTP request to AWS STS
			req, err := http.NewRequest(signingData.Method, stsURL, bytes.NewReader(bodyBytes))
			require.NoError(t, err)

			// Set all headers from the signed request
			for key, values := range headers {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}

			// Execute the request
			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Read the response
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			t.Logf("Response status: %d", resp.StatusCode)
			t.Logf("Response body: %s", string(respBody))

			// Verify the request succeeded
			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(respBody))
			}

			// Verify the response contains expected fields
			assert.Contains(t, string(respBody), "GetCallerIdentityResponse")
			assert.Contains(t, string(respBody), "UserId")
			assert.Contains(t, string(respBody), "Account")
			assert.Contains(t, string(respBody), "Arn")
		})
	}
}

// TestGenerateAwsAuthDataIntegrationDebug is a more verbose version that prints detailed signing information
// This helps debug signature mismatches.
//
// Run with: aws-vault exec sso-build-stable-developer -- go test -v -tags=integration ./comp/core/delegatedauth/api/cloudauth/... -run TestGenerateAwsAuthDataIntegrationDebug
func TestGenerateAwsAuthDataIntegrationDebug(t *testing.T) {
	// Check that AWS credentials are available
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKeyID == "" || secretAccessKey == "" {
		t.Skip("AWS credentials not available (AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY required)")
	}

	t.Logf("Using AWS Access Key ID: %s...", accessKeyID[:10])
	if sessionToken != "" {
		t.Logf("Session token present: %s...", sessionToken[:20])
	} else {
		t.Log("No session token present")
	}

	// Use a test org UUID
	orgUUID := "test-org-uuid-12345"

	// Create credentials
	awsCreds := &creds.SecurityCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Token:           sessionToken,
	}

	auth := &AWSAuth{
		AwsRegion: "", // Use default region
	}

	// Generate the signing data
	signingData, err := auth.generateAwsAuthData(orgUUID, awsCreds)
	require.NoError(t, err)
	require.NotNil(t, signingData)

	// Decode and display all components
	headersJSON, err := base64.StdEncoding.DecodeString(signingData.HeadersEncoded)
	require.NoError(t, err)

	var headers map[string][]string
	err = json.Unmarshal(headersJSON, &headers)
	require.NoError(t, err)

	bodyBytes, err := base64.StdEncoding.DecodeString(signingData.BodyEncoded)
	require.NoError(t, err)

	urlBytes, err := base64.StdEncoding.DecodeString(signingData.URLEncoded)
	require.NoError(t, err)

	fmt.Println("\n=== Signed Request Details ===")
	fmt.Printf("Method: %s\n", signingData.Method)
	fmt.Printf("URL: %s\n", string(urlBytes))
	fmt.Printf("Body: %s\n", string(bodyBytes))
	fmt.Println("\nHeaders:")
	for key, values := range headers {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
	fmt.Println("==============================")

	// Make the actual HTTP request to AWS STS
	stsURL := string(urlBytes)
	req, err := http.NewRequest(signingData.Method, stsURL, bytes.NewReader(bodyBytes))
	require.NoError(t, err)

	// Set all headers from the signed request
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	fmt.Printf("\nResponse Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Headers:\n")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
	fmt.Printf("\nResponse Body:\n%s\n", string(respBody))

	// Check the result
	if resp.StatusCode != 200 {
		t.Errorf("Request failed with status %d", resp.StatusCode)
	}
}

// TestGenerateAwsAuthDataWithoutOrgHeader tests if the request works WITHOUT the custom org header
// This helps isolate whether the custom header is causing the signature issue
//
// Run with: aws-vault exec sso-build-stable-developer -- go test -v -tags=integration ./comp/core/delegatedauth/api/cloudauth/... -run TestGenerateAwsAuthDataWithoutOrgHeader
func TestGenerateAwsAuthDataWithoutOrgHeader(t *testing.T) {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKeyID == "" || secretAccessKey == "" {
		t.Skip("AWS credentials not available")
	}

	t.Logf("Testing WITHOUT org header")
	t.Logf("Access Key: %s...", accessKeyID[:10])

	// For now, just verify our credentials work with a basic AWS SDK call
	// We'll use the generateAwsAuthData but then remove the org header before sending
	auth := &AWSAuth{AwsRegion: ""}
	credsPtr := &creds.SecurityCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Token:           sessionToken,
	}

	signingData, err := auth.generateAwsAuthData("test-org", credsPtr)
	require.NoError(t, err)

	// Decode headers
	headersJSON, err := base64.StdEncoding.DecodeString(signingData.HeadersEncoded)
	require.NoError(t, err)

	var headers map[string][]string
	err = json.Unmarshal(headersJSON, &headers)
	require.NoError(t, err)

	// Remove the org header
	delete(headers, "X-Ddog-Org-Id")
	t.Log("Removed X-Ddog-Org-Id header from request")

	// Decode body and URL
	bodyBytesNew, err := base64.StdEncoding.DecodeString(signingData.BodyEncoded)
	require.NoError(t, err)

	urlBytes, err := base64.StdEncoding.DecodeString(signingData.URLEncoded)
	require.NoError(t, err)

	// Make request
	reqNew, err := http.NewRequest("POST", string(urlBytes), bytes.NewReader(bodyBytesNew))
	require.NoError(t, err)

	for key, values := range headers {
		for _, value := range values {
			reqNew.Header.Add(key, value)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(reqNew)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	t.Logf("Response status: %d", resp.StatusCode)
	t.Logf("Response: %s", string(respBody))

	// This request should also fail because we modified the signature
	// by removing a signed header
	if resp.StatusCode != 200 {
		t.Logf("Expected failure: removing signed header breaks signature (status %d)", resp.StatusCode)
	}
}
