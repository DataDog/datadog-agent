// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

// Fixed inputs for the signature golden. Credentials are the AWS documentation examples plus a
// dummy session token; they are not real.
const (
	goldenAccessKeyID     = "AKIAIOSFODNN7EXAMPLE"
	goldenSecretAccessKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	goldenSessionToken    = "FwoGZXIvYXdzEExampleSessionToken"
	goldenOrgUUID         = "11111111-2222-3333-4444-555555555555"

	// goldenAuthorization is the Authorization header aws-sdk-go-v2's signer
	// (aws/signer/v4, v1.43.3) produced for this exact request before the switch to
	// smithy-go/aws-http-auth. Regenerate it with the SDK signer, never with the current one:
	// a golden recomputed with the code under test would assert nothing.
	goldenAuthorization = "AWS4-HMAC-SHA256 " +
		"Credential=AKIAIOSFODNN7EXAMPLE/20260805/us-east-1/sts/aws4_request, " +
		"SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-security-token;x-ddog-org-id, " +
		"Signature=3c567cb74f09e429570dae6682df7b08d094e3025394a08df8e92ab225fc2006"
)

var goldenSigningTime = time.Date(2026, 8, 5, 12, 34, 56, 0, time.UTC)

// TestSignedProofMatchesAWSSDK pins the proof's signature to the one aws-sdk-go-v2 produced.
// aws-http-auth signs only Host and X-Amz-* by default, which would silently drop x-ddog-org-id
// from the signature and let the org a proof is issued for be changed in transit; signedHeaderRules
// exists to prevent that. A byte-for-byte match is the check that the replacement signer, the
// header rules and the raw (not hex) payload hash together reproduce the previous signature, so
// proofs the backend already accepts keep validating.
func TestSignedProofMatchesAWSSDK(t *testing.T) {
	original := signingTime
	signingTime = func() time.Time { return goldenSigningTime }
	t.Cleanup(func() { signingTime = original })

	auth := NewAWSAuth(nil) // no region: global STS endpoint, us-east-1
	data, err := auth.generateAwsAuthData(context.Background(), goldenOrgUUID, &creds.SecurityCredentials{
		AccessKeyID:     goldenAccessKeyID,
		SecretAccessKey: goldenSecretAccessKey,
		Token:           goldenSessionToken,
	})
	require.NoError(t, err)

	headers := decodeProofHeaders(t, data.headersEncoded)
	require.Contains(t, headers, "Authorization")
	assert.Equal(t, goldenAuthorization, headers["Authorization"][0],
		"signature changed; the backend validates this proof, so a diff here is a breaking change")

	assert.Equal(t, []string{"20260805T123456Z"}, headers["X-Amz-Date"])
	assert.Equal(t, []string{goldenSessionToken}, headers["X-Amz-Security-Token"])

	// The proof's header blob is parsed by the backend, so its key set is part of the contract and
	// not just the signature. content-length is signed but deliberately not carried: aws-sdk-go-v2
	// never put it in the map either, and the backend reconstructs it on replay. Asserting the exact
	// key set is what catches a signer change that starts adding or dropping headers.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{
		"Authorization",
		"Content-Type",
		"Host",
		"User-Agent",
		"X-Amz-Date",
		"X-Amz-Security-Token",
		"X-Ddog-Org-Id",
	}, keys)
}

// TestSignedHeaderRulesMatchAWSSDKExcludeList documents the header set the signature covers.
// Anything the Agent sets on the STS request is signed except the five aws-sdk-go-v2 excludes,
// so a header added later is covered by default rather than silently left unprotected.
func TestSignedHeaderRulesMatchAWSSDKExcludeList(t *testing.T) {
	for _, h := range []string{"authorization", "user-agent", "x-amzn-trace-id", "expect", "transfer-encoding"} {
		assert.False(t, signedHeaderRules{}.IsSigned(h), "%s must not be signed", h)
	}
	for _, h := range []string{"host", "content-type", "content-length", "x-ddog-org-id", "x-amz-date", "x-amz-security-token"} {
		assert.True(t, signedHeaderRules{}.IsSigned(h), "%s must be signed", h)
	}
}

func decodeProofHeaders(t *testing.T, encoded string) map[string][]string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	var headers map[string][]string
	require.NoError(t, json.Unmarshal(raw, &headers))
	return headers
}
