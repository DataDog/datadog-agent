// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
)

// stubCredentialStore is a credentialStore backed by a map.
type stubCredentialStore struct {
	creds   map[string]credentials.Credential
	loadErr error

	mu        sync.Mutex
	loadCalls int
}

func (s *stubCredentialStore) Load() (map[string]credentials.Credential, error) {
	s.mu.Lock()
	s.loadCalls++
	s.mu.Unlock()
	return s.creds, s.loadErr
}

func (s *stubCredentialStore) loads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCalls
}

func TestResolveCredentials(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-a": {ID: "cred-a", SNMPVersion: "2c", CommunityString: "public"},
		"cred-b": {ID: "cred-b", SNMPVersion: "3", User: "datadog", ContextEngineID: "engine"},
	}}

	creds, err := resolveCredentials(store, []string{"cred-a", "cred-b"})
	require.NoError(t, err)
	require.Len(t, creds, 2)
	assert.Equal(t, "cred-a", creds[0].ID)
	assert.Equal(t, "2c", creds[0].Version)
	assert.Equal(t, "public", creds[0].Community)
	assert.Equal(t, "cred-b", creds[1].ID)
	assert.Equal(t, "engine", creds[1].ContextEngineID)
}

func TestResolveCredentialsMissingID(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-a": {ID: "cred-a", SNMPVersion: "2c"},
	}}

	_, err := resolveCredentials(store, []string{"cred-a", "nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestResolveCredentialsRejectsUnknownVersion(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"bad": {ID: "bad", SNMPVersion: "4"},
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

func TestResolveCredentialsSurfacesALoadFailure(t *testing.T) {
	store := &stubCredentialStore{loadErr: errors.New("boom")}
	_, err := resolveCredentials(store, []string{"cred-a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestResolveCredentialsRejectsUnknownV3Protocols(t *testing.T) {
	tests := []struct {
		name    string
		cred    credentials.Credential
		errPart string
	}{
		{
			name:    "bad auth protocol",
			cred:    credentials.Credential{ID: "bad", SNMPVersion: "3", User: "datadog", AuthProtocol: "sha-256", AuthKey: "secret"},
			errPart: "authProtocol",
		},
		{
			name:    "bad priv protocol",
			cred:    credentials.Credential{ID: "bad", SNMPVersion: "3", User: "datadog", PrivProtocol: "aes-256", PrivKey: "secret"},
			errPart: "privProtocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubCredentialStore{creds: map[string]credentials.Credential{"bad": tt.cred}}
			_, err := resolveCredentials(store, []string{"bad"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
			assert.NotContains(t, err.Error(), "secret", "an error must never carry key material")
		})
	}
}

func TestResolveCredentialsAcceptsEveryValidV3Protocol(t *testing.T) {
	auths := []string{"", "md5", "sha", "sha224", "sha256", "sha384", "sha512", "SHA256"}
	privs := []string{"", "des", "aes", "aes192", "aes256", "aes192c", "aes256c", "AES256C"}

	for _, a := range auths {
		for _, p := range privs {
			store := &stubCredentialStore{creds: map[string]credentials.Credential{
				"cred": {ID: "cred", SNMPVersion: "3", User: "datadog", AuthProtocol: a, PrivProtocol: p},
			}}
			_, err := resolveCredentials(store, []string{"cred"})
			require.NoErrorf(t, err, "auth=%q priv=%q must be accepted", a, p)
		}
	}
}

func TestResolveCredentialsIgnoresProtocolsOnV2c(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred": {ID: "cred", SNMPVersion: "2c", CommunityString: "public", AuthProtocol: "sha-256", PrivProtocol: "aes-256"},
	}}
	_, err := resolveCredentials(store, []string{"cred"})
	require.NoError(t, err)
}
