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
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
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

func TestNewIdentityStorage(t *testing.T) {
	tests := []struct {
		name      string
		flavor    string
		useK8s    bool
		wantStore IdentityStorage
	}{
		{"node agent uses file", flavor.DefaultAgent, false, &fileIdentityStorage{}},
		{"node agent ignores k8s secret setting", flavor.DefaultAgent, true, &fileIdentityStorage{}},
		{"cluster agent without k8s secret uses file", flavor.ClusterAgent, false, &fileIdentityStorage{}},
		{"cluster agent with k8s secret uses secret", flavor.ClusterAgent, true, &k8sIdentityStorage{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor.SetTestFlavor(t, tt.flavor)
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				par.IdentityUseK8sSecret: tt.useK8s,
			})

			assert.IsType(t, tt.wantStore, NewIdentityStorage(cfg, nil))
		})
	}
}
