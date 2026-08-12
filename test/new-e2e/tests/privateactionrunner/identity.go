// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/require"
)

const testRunnerURN = "urn:dd:apps:on-prem-runner:us1:123456:test-runner-e2e"

// GenerateTestRunnerIdentity generates a fresh ECDSA key pair and returns the
// runner URN and base64-encoded private JWK for agent config or Helm values.
func GenerateTestRunnerIdentity(t *testing.T) (urn string, privateKeyB64 string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "failed to generate ECDSA key")

	privateJwk := jose.JSONWebKey{
		Algorithm: "ES256",
		Key:       privateKey,
		Use:       "sig",
	}
	jwkJSON, err := json.Marshal(privateJwk)
	require.NoError(t, err, "failed to marshal JWK")

	return testRunnerURN, base64.RawURLEncoding.EncodeToString(jwkJSON)
}

// GenerateTestPrivateActionRunnerConfig returns datadog.yaml snippet with PAR enabled
// and a valid test identity so the runner can start without self-enrollment.
func GenerateTestPrivateActionRunnerConfig(t *testing.T) string {
	t.Helper()
	urn, privateKeyB64 := GenerateTestRunnerIdentity(t)
	return fmt.Sprintf(`private_action_runner:
  enabled: true
  private_key: %s
  urn: %s
`, privateKeyB64, urn)
}
