// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// stubCredentialStore is a credentialStore backed by a map, used by the tests
// in this package that need credentials without touching the agent config.
type stubCredentialStore struct {
	creds       map[string]connectivity.SNMPCredential
	reloadCalls int
	reloadErr   error
}

func (s *stubCredentialStore) Reload() error {
	s.reloadCalls++
	return s.reloadErr
}

func (s *stubCredentialStore) Get(id string) (connectivity.SNMPCredential, bool) {
	c, ok := s.creds[id]
	return c, ok
}

func TestConfigCredentialStoreReadsAgentConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("network_devices.discovery.credentials", []interface{}{
		map[string]interface{}{
			"id":               "cred-a",
			"snmp_version":     "2c",
			"community_string": "public",
		},
		map[string]interface{}{
			"id":           "cred-b",
			"snmp_version": "3",
			"user":         "datadog",
			"authProtocol": "SHA",
			"authKey":      "auth-key",
			"privProtocol": "AES",
			"privKey":      "priv-key",
			"context_name": "ctx",
		},
	})

	store := newConfigCredentialStore(cfg)
	require.NoError(t, store.Reload())

	a, ok := store.Get("cred-a")
	require.True(t, ok)
	assert.Equal(t, "2c", a.Version)
	assert.Equal(t, "public", a.Community)
	assert.Equal(t, "cred-a", a.ID)

	b, ok := store.Get("cred-b")
	require.True(t, ok)
	assert.Equal(t, "3", b.Version)
	assert.Equal(t, "datadog", b.User)
	assert.Equal(t, "SHA", b.AuthProtocol)
	assert.Equal(t, "auth-key", b.AuthKey)
	assert.Equal(t, "AES", b.PrivProtocol)
	assert.Equal(t, "priv-key", b.PrivKey)
	assert.Equal(t, "ctx", b.ContextName)

	_, ok = store.Get("missing")
	assert.False(t, ok)
}

func TestConfigCredentialStoreReloadPicksUpChanges(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("network_devices.discovery.credentials", []interface{}{
		map[string]interface{}{"id": "cred-a", "snmp_version": "2c", "community_string": "old"},
	})

	store := newConfigCredentialStore(cfg)
	require.NoError(t, store.Reload())
	a, _ := store.Get("cred-a")
	assert.Equal(t, "old", a.Community)

	cfg.SetInTest("network_devices.discovery.credentials", []interface{}{
		map[string]interface{}{"id": "cred-a", "snmp_version": "2c", "community_string": "new"},
	})
	require.NoError(t, store.Reload())
	a, _ = store.Get("cred-a")
	assert.Equal(t, "new", a.Community)
}

func TestConfigCredentialStoreEmptyConfig(t *testing.T) {
	cfg := configmock.New(t)
	store := newConfigCredentialStore(cfg)
	require.NoError(t, store.Reload())
	_, ok := store.Get("anything")
	assert.False(t, ok)
}

func TestResolveCredentials(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"cred-a": {ID: "cred-a", Version: "2c", Community: "public"},
		"cred-b": {ID: "cred-b", Version: "3", User: "datadog"},
		"bad":    {ID: "bad", Version: "4"},
	}}

	creds, err := resolveCredentials(store, []string{"cred-a", "cred-b"})
	require.NoError(t, err)
	require.Len(t, creds, 2)
	assert.Equal(t, "cred-a", creds[0].ID)
	assert.Equal(t, "cred-b", creds[1].ID)
}

func TestResolveCredentialsMissingID(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"cred-a": {ID: "cred-a", Version: "2c"},
	}}

	_, err := resolveCredentials(store, []string{"cred-a", "nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestResolveCredentialsRejectsUnknownVersion(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"bad": {ID: "bad", Version: "4"},
	}}

	_, err := resolveCredentials(store, []string{"bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestResolveCredentialsRejectsEmptyList(t *testing.T) {
	store := &stubCredentialStore{}
	_, err := resolveCredentials(store, nil)
	require.Error(t, err)
}
