// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"crypto/ecdsa"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func TestPersistIdentityToFile_RoundTripsAPIKeyHash(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	})

	privateJwk, _, err := util.GenerateKeys()
	require.NoError(t, err)
	privateKey := privateJwk.Key.(*ecdsa.PrivateKey)

	result := &Result{
		PrivateKey: privateKey,
		URN:        "urn:dd:runner:us1:123:test-runner",
		Hostname:   "my-host",
		APIKeyHash: HashAPIKey("my-api-key"),
	}

	require.NoError(t, persistIdentityToFile(cfg, result))

	identity, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, HashAPIKey("my-api-key"), identity.APIKeyHash)
	assert.Equal(t, "my-host", identity.Hostname)
	assert.Equal(t, result.URN, identity.URN)
}

func TestPersistIdentityToFile_NoAPIKeyHash(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	})

	privateJwk, _, err := util.GenerateKeys()
	require.NoError(t, err)
	privateKey := privateJwk.Key.(*ecdsa.PrivateKey)

	result := &Result{
		PrivateKey: privateKey,
		URN:        "urn:dd:runner:us1:123:test-runner",
		Hostname:   "my-host",
	}

	require.NoError(t, persistIdentityToFile(cfg, result))

	identity, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Empty(t, identity.APIKeyHash)
}
