// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package bootstrapparcontrol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func TestEnsureEnrollmentPersistedIdentity(t *testing.T) {
	tests := []struct {
		name             string
		identityHostname string
		wantEnroll       bool
	}{
		{name: "current hostname", identityHostname: "test-host"},
		{name: "legacy identity without hostname"},
		{name: "stale hostname", identityHostname: "old-host", wantEnroll: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			identityPath := filepath.Join(t.TempDir(), "identity.json")
			writeIdentity(t, identityPath, tc.identityHostname)
			cfg := testConfig(t, identityPath, map[string]interface{}{
				"private_action_runner.self_enroll": true,
			})
			hostnameComp, _ := hostnamemock.NewMock("test-host")
			enrollCalls := 0

			err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, func(_ context.Context, _ log.Component, _ coreconfig.Component, _ *enrollment.AgentIdentifier) (*enrollment.Result, error) {
				enrollCalls++
				return &enrollment.Result{URN: validURN()}, nil
			})

			require.NoError(t, err)
			if tc.wantEnroll {
				assert.Equal(t, 1, enrollCalls)
			} else {
				assert.Zero(t, enrollCalls)
			}
		})
	}
}

func TestEnsureEnrollmentCorruptIdentityFallsBack(t *testing.T) {
	t.Run("self-enrolls", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		require.NoError(t, os.WriteFile(identityPath, []byte("not-json"), 0o600))
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": true,
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")
		enrollCalls := 0

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, func(_ context.Context, _ log.Component, _ coreconfig.Component, _ *enrollment.AgentIdentifier) (*enrollment.Result, error) {
			enrollCalls++
			return &enrollment.Result{URN: validURN()}, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 1, enrollCalls)
	})

	t.Run("configured identity wins and the file is removed", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		require.NoError(t, os.WriteFile(identityPath, []byte("not-json"), 0o600))
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": false,
			"private_action_runner.urn":         validURN(),
			"private_action_runner.private_key": validPrivateKey(t),
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.NoError(t, err)
		assert.NoFileExists(t, identityPath)
	})
}

func TestEnsureEnrollmentMissingIdentityEnrolls(t *testing.T) {
	identityPath := filepath.Join(t.TempDir(), "missing.json")
	cfg := testConfig(t, identityPath, map[string]interface{}{
		"private_action_runner.self_enroll": true,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	enrollCalls := 0

	err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, func(_ context.Context, _ log.Component, _ coreconfig.Component, agentID *enrollment.AgentIdentifier) (*enrollment.Result, error) {
		enrollCalls++
		assert.Equal(t, "test-host", agentID.Hostname)
		return &enrollment.Result{URN: validURN()}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, enrollCalls)
}

func TestEnsureEnrollmentConfiguredIdentity(t *testing.T) {
	t.Run("missing persisted identity", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "missing.json")
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": false,
			"private_action_runner.urn":         validURN(),
			"private_action_runner.private_key": validPrivateKey(t),
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.NoError(t, err)
		assert.NoFileExists(t, identityPath)
	})

	t.Run("stale persisted identity is removed", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		writeIdentity(t, identityPath, "old-host")
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": false,
			"private_action_runner.urn":         validURN(),
			"private_action_runner.private_key": validPrivateKey(t),
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.NoError(t, err)
		assert.NoFileExists(t, identityPath)
	})

	t.Run("valid persisted identity takes precedence", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		writeIdentity(t, identityPath, "test-host")
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": false,
			"private_action_runner.urn":         "invalid-inline-urn",
			"private_action_runner.private_key": "invalid-inline-key",
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.NoError(t, err)
		assert.FileExists(t, identityPath)
	})
}

func TestEnsureEnrollmentFailures(t *testing.T) {
	t.Run("self enrollment disabled", func(t *testing.T) {
		cfg := testConfig(t, filepath.Join(t.TempDir(), "missing.json"), map[string]interface{}{
			"private_action_runner.self_enroll": false,
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "private_action_runner.self_enroll is false")
	})

	t.Run("corrupt persisted identity without any fallback", func(t *testing.T) {
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		require.NoError(t, os.WriteFile(identityPath, []byte("not-json"), 0o600))
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": false,
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "private_action_runner.self_enroll is false")
	})

	t.Run("unreadable persisted identity is not discarded", func(t *testing.T) {
		// A directory stands in for any I/O failure.
		identityPath := filepath.Join(t.TempDir(), "identity.json")
		require.NoError(t, os.Mkdir(identityPath, 0o700))
		cfg := testConfig(t, identityPath, map[string]interface{}{
			"private_action_runner.self_enroll": true,
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load persisted identity")
	})

	t.Run("invalid configured private key", func(t *testing.T) {
		cfg := testConfig(t, filepath.Join(t.TempDir(), "missing.json"), map[string]interface{}{
			"private_action_runner.self_enroll": true,
			"private_action_runner.private_key": "sensitive-invalid-key",
		})
		hostnameComp, _ := hostnamemock.NewMock("test-host")

		err := ensureEnrollment(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "private_action_runner.private_key is invalid")
		assert.NotContains(t, err.Error(), "sensitive-invalid-key")
	})
}

func testConfig(t *testing.T, identityPath string, overrides map[string]interface{}) coreconfig.Component {
	values := map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.identity_file_path": identityPath,
	}
	for key, value := range overrides {
		values[key] = value
	}
	return coreconfig.NewMockWithOverrides(t, values)
}

func writeIdentity(t *testing.T, path, hostname string) {
	data, err := json.Marshal(enrollment.PersistedIdentity{
		URN:        validURN(),
		PrivateKey: validPrivateKey(t),
		Hostname:   hostname,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func validURN() string {
	return parutil.MakeRunnerURN("us1", 123, "test-runner")
}

func validPrivateKey(t *testing.T) string {
	privateJWK, _, err := parutil.GenerateKeys()
	require.NoError(t, err)
	encoded, err := privateJWK.MarshalJSON()
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func failIfEnrolled(t *testing.T) enrollAndPersistFunc {
	return func(context.Context, log.Component, coreconfig.Component, *enrollment.AgentIdentifier) (*enrollment.Result, error) {
		t.Fatal("unexpected enrollment")
		return nil, nil
	}
}
