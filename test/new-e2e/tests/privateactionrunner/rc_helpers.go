// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// pkg/remoteconfig/state.ProductActionPlatformRunnerKeys
const runnerKeysRCProduct = "AP_RUNNER_KEYS"

// mirrors pkg/privateactionrunner/types.RawKey's JSON shape
type rawRCKey struct {
	KeyType string `json:"keyType"`
	Key     []byte `json:"key"`
}

// PushFakeRunnerKeysConfig generates a fresh ED25519 key pair and pushes its
// public key to fakeintake as an AP_RUNNER_KEYS remote-config update, letting
// a PAR under test complete KeysManager startup without a real backend.
func PushFakeRunnerKeysConfig(t *testing.T, client *fakeintakeclient.Client) {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "failed to generate fake runner key")

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err, "failed to marshal fake runner public key")

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	payload, err := json.Marshal(rawRCKey{KeyType: "ED25519", Key: pubPEM})
	require.NoError(t, err, "failed to marshal fake runner key config payload")

	err = client.RCAddConfig("", runnerKeysRCProduct, "fake-runner-key", "fake-runner-key", payload)
	require.NoError(t, err, "failed to push fake runner key config to fakeintake")
}
