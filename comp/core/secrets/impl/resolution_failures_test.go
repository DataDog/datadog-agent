// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretsimpl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
)

func TestResolutionFailures(t *testing.T) {
	for _, tc := range []struct {
		name, response, reason string
		err                    error
	}{
		{"missing", `{"good":{"value":"resolved-good"}}`, "missing", nil},
		{"empty", `{"good":{"value":"resolved-good"},"bad":{"value":""}}`, "empty", nil},
		{"backend error", `{"good":{"value":"resolved-good"},"bad":{"error":"sensitive backend text"}}`, "backend_error", nil},
		{"invalid response", `{`, "invalid_response", nil},
		{"command failure", "", "backend_error", errors.New("sensitive execution error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
			r.commandHookFunc = func(string) ([]byte, error) { return []byte(tc.response), tc.err }
			r.SetOriginConfig("config-id", "redis", "file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml")
			_, err := r.Resolve([]byte("password: ENC[bad]\nusername: ENC[good]\n"), "config-id", "", "", false)
			require.Error(t, err)
			failures := r.GetResolutionFailures()
			require.NotEmpty(t, failures)
			for _, failure := range failures {
				assert.Equal(t, "config-id", failure.Origin)
				assert.Equal(t, "redis", failure.OriginName)
				assert.Equal(t, "file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml", failure.ConfigSource)
				assert.Equal(t, tc.reason, failure.Reason)
				assert.False(t, failure.HasCachedValue)
			}
			if tc.err == nil && tc.reason != "invalid_response" {
				require.Len(t, failures, 1)
				assert.Equal(t, "bad", failures[0].Handle)
				assert.Equal(t, []string{"password"}, failures[0].Path)
				assert.Equal(t, "resolved-good", r.cache["good"])
			}
		})
	}
}

func TestResolutionFailureRecoveryAndRemoval(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	response := `{}`
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(response), nil }
	config := []byte("password: ENC[bad]\n")
	for _, origin := range []string{"first", "second", "first"} {
		_, err := r.Resolve(config, origin, "", "", false)
		require.Error(t, err)
	}
	require.Len(t, r.GetResolutionFailures(), 2)
	failures := r.GetResolutionFailures()
	failures[0].Path[0] = "modified"
	assert.Equal(t, []string{"password"}, r.GetResolutionFailures()[0].Path)
	r.RemoveOrigin("first")
	require.Len(t, r.GetResolutionFailures(), 1)
	assert.Equal(t, "second", r.GetResolutionFailures()[0].Origin)
	response = `{"bad":{"value":"ENC[literal-value]"}}`
	_, err := r.RefreshNow()
	require.NoError(t, err)
	assert.Empty(t, r.GetResolutionFailures())
	assert.Equal(t, "ENC[literal-value]", r.cache["bad"])
	response = `{}`
	_, err = r.RefreshNow()
	require.Error(t, err)
	require.Len(t, r.GetResolutionFailures(), 1)
	r.RemoveOrigin("second")
	assert.Empty(t, r.GetResolutionFailures())
}

func TestResolutionFailureAmbiguousSource(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(`{}`), nil }
	for _, source := range []string{"file:/first/conf.yaml", "file:/second/conf.yaml", "file:/first/conf.yaml"} {
		r.SetOriginConfig("same-digest", "redis", source)
		_, err := r.Resolve([]byte("password: ENC[missing]\n"), "same-digest", "", "", false)
		require.Error(t, err)
	}
	assert.Empty(t, r.GetResolutionFailures()[0].ConfigSource, "an ambiguous file must not receive an inline warning")
}

func TestRefreshFailureClearsOnUnchangedValue(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	var notified []secrets.ResolutionFailure
	r.SetResolutionFailureCallback(func(failures []secrets.ResolutionFailure, _ bool) { notified = failures })
	response := `{"password":{"value":"same-value"}}`
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(response), nil }
	config := []byte("password: ENC[password]\n")
	_, err := r.Resolve(config, "redis", "", "", false)
	require.NoError(t, err)
	response = `{"password":{"error":"lookup failed"}}`
	_, err = r.RefreshNow()
	require.Error(t, err)
	require.Len(t, notified, 1, "failed lookups must notify the reporter immediately")
	assert.True(t, notified[0].HasCachedValue)
	_, err = r.Resolve(config, "redis", "", "", false)
	require.NoError(t, err)
	require.Len(t, notified, 1, "using the cache is not a successful backend lookup")
	response = `{"password":{"value":"same-value"}}`
	_, err = r.RefreshNow()
	require.NoError(t, err)
	assert.Empty(t, notified, "successful unchanged values must notify recovery")
}

func TestResolutionReporterStartupAndRemoval(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(`{}`), nil }
	_, err := r.Resolve([]byte("password: ENC[missing]\n"), "redis", "", "", false)
	require.Error(t, err)
	var notified []secrets.ResolutionFailure
	var ready bool
	r.SetResolutionFailureCallback(func(failures []secrets.ResolutionFailure, initialLoadComplete bool) {
		notified, ready = failures, initialLoadComplete
	})
	require.Len(t, notified, 1, "registration must replay failures recorded before Health started")
	assert.False(t, ready)
	r.CompleteInitialResolution()
	assert.True(t, ready)
	r.RemoveOrigin("redis")
	assert.Empty(t, notified, "removing the configuration must notify the reporter")
}

func TestResolutionFailureMultiBackendHandles(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	r.multiBackends = map[string]secrets.SecretBackendConfig{"file": {Type: "file.yaml"}}
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(`{"good":{"value":"resolved"}}`), nil }
	_, err := r.Resolve([]byte("username: ENC[file;good]\npassword: ENC[file;missing]\n"), "redis", "", "", false)
	require.Error(t, err)
	require.Len(t, r.GetResolutionFailures(), 1)
	assert.Equal(t, "file;missing", r.GetResolutionFailures()[0].Handle)
	assert.Equal(t, "resolved", r.cache["file;good"])
}

func TestSkippedLookupsHaveNoResolutionFailure(t *testing.T) {
	r := newResolver(t, secrets.ConfigParams{})
	_, err := r.Resolve([]byte("password: ENC[bad]\n"), "redis", "", "", false)
	require.NoError(t, err)
	assert.Empty(t, r.GetResolutionFailures())
	r.secretBackendMethod = "secret_backend_command"
	r.scopeIntegrationToNamespace = true
	_, err = r.Resolve([]byte("password: ENC[other/secret;key]\n"), "redis", "", "current", false)
	require.NoError(t, err)
	assert.Empty(t, r.GetResolutionFailures())
}

func TestAuditFailureIsNotResolutionFailure(t *testing.T) {
	r := newEnabledSecretResolver(nooptelemetry.GetCompatComponent())
	response := `{"password":{"value":"original"}}`
	r.commandHookFunc = func(string) ([]byte, error) { return []byte(response), nil }
	_, err := r.Resolve([]byte("password: ENC[password]\n"), "redis", "", "", false)
	require.NoError(t, err)
	r.auditFilename = t.TempDir()
	response = `{"password":{"value":"updated"}}`
	_, err = r.RefreshNow()
	require.Error(t, err)
	assert.Empty(t, r.GetResolutionFailures())
	assert.Equal(t, "updated", r.cache["password"])
}
