// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"path/filepath"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/stretchr/testify/require"
)

func TestDisabledExecutorDoesNotResolveIdentityOrInitializeActions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"private_action_runner.enabled":       enabled,
			"private_action_runner.split_enabled": false,
			"private_action_runner.self_enroll":   true,
			"private_action_runner.private_key":   "invalid-key",
		})
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "false")
		// Dependencies are deliberately absent: disabled startup must not use them.
		runner := &PrivateActionRunner{coreConfig: cfg}
		_, resolved, err := runner.configureExecutor(context.Background(), context.Background())
		require.NoError(t, err)
		require.Nil(t, resolved)
		require.NotNil(t, runner.executorServer)
		require.Nil(t, runner.encryptionStore)
		require.Nil(t, runner.keysManager)
	}
}

func TestExecutorSnapshotUsesMonolithPersistedIdentity(t *testing.T) {
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	encoded, err := key.MarshalJSON()
	require.NoError(t, err)
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.urn":                util.MakeRunnerURN("us1", 123, "configured-runner"),
		"private_action_runner.private_key":        base64.RawURLEncoding.EncodeToString(encoded),
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	})
	persistedKey, _, err := util.GenerateKeys()
	require.NoError(t, err)
	persisted := &enrollment.Result{
		URN:        util.MakeRunnerURN("us1", 42, "persisted-runner"),
		PrivateKey: persistedKey.Key.(*ecdsa.PrivateKey), Hostname: "test-host",
	}
	require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, persisted))
	hostname, _ := hostnamemock.NewMock("test-host")
	runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname}
	resolved, err := runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, persisted.URN, resolved.Urn)
	snapshot, err := executor.ControlPlaneConfig(cfg, resolved)
	require.NoError(t, err)
	require.Equal(t, persisted.URN, snapshot.Identity.Urn)
	snapshotKey, err := util.Base64ToJWK(snapshot.Identity.PrivateKey)
	require.NoError(t, err)
	require.True(t, persisted.PrivateKey.Equal(snapshotKey.Key))
}
