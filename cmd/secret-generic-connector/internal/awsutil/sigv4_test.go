// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package awsutil

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignRequest(t *testing.T) {
	creds := aws.Credentials{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}

	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req, err := http.NewRequest("POST", "https://sts.amazonaws.com/", strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fixedTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	signRequestAt(req, creds, "us-east-1", "sts", body, fixedTime)

	authHeader := req.Header.Get("Authorization")
	assert.True(t, strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20240115/us-east-1/sts/aws4_request"))
	assert.Contains(t, authHeader, "SignedHeaders=")
	assert.Contains(t, authHeader, "Signature=")
	assert.Equal(t, "20240115T120000Z", req.Header.Get("X-Amz-Date"))
}

func TestSignRequestWithSessionToken(t *testing.T) {
	creds := aws.Credentials{
		AccessKeyID:     "AKID",
		SecretAccessKey: "SECRET",
		SessionToken:    "TOKEN123",
	}

	req, err := http.NewRequest("GET", "https://example.com/test", nil)
	require.NoError(t, err)

	fixedTime := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	signRequestAt(req, creds, "us-west-2", "s3", nil, fixedTime)

	assert.Equal(t, "TOKEN123", req.Header.Get("X-Amz-Security-Token"))
	assert.NotEmpty(t, req.Header.Get("Authorization"))
}

func TestDeriveSigningKey(t *testing.T) {
	key := deriveSigningKey("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "20240115", "us-east-1", "sts")
	assert.Len(t, key, 32)
}

func TestCanonicalQueryString(t *testing.T) {
	tests := []struct {
		rawURL   string
		expected string
	}{
		{"https://example.com/", ""},
		{"https://example.com/?b=2&a=1", "a=1&b=2"},
		{"https://example.com/?key=val%20ue", "key=val%20ue"},
	}
	for _, tt := range tests {
		u, err := http.NewRequest("GET", tt.rawURL, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, canonicalQueryString(u.URL))
	}
}
