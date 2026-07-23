// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privateactionrunner contains e2e tests for the Private Action Runner rshell bundle.
package privateactionrunner

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRunnerOrgID and testRunnerRunnerID are the org_id/runner_id encoded in testRunnerURN
// (defined in identity.go).
const (
	testRunnerOrgID    = 123456
	testRunnerRunnerID = "test-runner-e2e"
)

// generateTestRunnerIdentity is a test-local alias for GenerateTestRunnerIdentity.
func generateTestRunnerIdentity(t *testing.T) (urn string, privateKeyB64 string) {
	return GenerateTestRunnerIdentity(t)
}

// generateTestSigningKey generates an ED25519 key pair for signing PAR tasks: the
// private half signs envelopes in fakeintake, the public half is pushed to PAR via RC.
func generateTestSigningKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "failed to generate ED25519 signing key")

	return pub, priv
}

// encodeED25519PublicKeyRC builds the AP_RUNNER_KEYS RC payload PAR expects: an
// X509-PEM-encoded public key tagged with its type (see types.RawKey).
func encodeED25519PublicKeyRC(pub ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	return json.Marshal(struct {
		KeyType string `json:"keyType"`
		Key     []byte `json:"key"`
	}{
		KeyType: "ED25519",
		Key:     keyPEM,
	})
}
