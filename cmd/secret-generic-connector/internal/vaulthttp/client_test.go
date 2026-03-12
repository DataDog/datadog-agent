// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package vaulthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:8200", nil)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8200", c.Address())
	assert.Equal(t, "", c.Token())

	c.SetToken("test-token")
	assert.Equal(t, "test-token", c.Token())
}

func TestRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/v1/secret/foo", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

		resp := map[string]interface{}{
			"request_id":     "abc",
			"lease_id":       "",
			"renewable":      false,
			"lease_duration": 2764800,
			"data": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, err := NewClient(server.URL, nil)
	require.NoError(t, err)
	c.SetToken("test-token")

	secret, err := c.Read(context.Background(), "secret/foo")
	require.NoError(t, err)
	assert.Equal(t, "abc", secret.RequestID)
	assert.Equal(t, "value1", secret.Data["key1"])
	assert.Equal(t, "value2", secret.Data["key2"])
}

func TestWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/auth/approle/login", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "test-role", body["role_id"])

		resp := map[string]interface{}{
			"auth": map[string]interface{}{
				"client_token": "new-token",
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, err := NewClient(server.URL, nil)
	require.NoError(t, err)

	secret, err := c.Write(context.Background(), "auth/approle/login", map[string]interface{}{
		"role_id":   "test-role",
		"secret_id": "test-secret",
	})
	require.NoError(t, err)

	token, err := secret.TokenID()
	require.NoError(t, err)
	assert.Equal(t, "new-token", token)
}

func TestListMounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/v1/sys/mounts", r.URL.Path)

		resp := map[string]interface{}{
			"request_id": "xyz",
			"secret/": map[string]interface{}{
				"type":    "kv",
				"options": map[string]string{"version": "1"},
			},
			"kv2/": map[string]interface{}{
				"type":    "kv",
				"options": map[string]string{"version": "2"},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, err := NewClient(server.URL, nil)
	require.NoError(t, err)
	c.SetToken("test-token")

	mounts, err := c.ListMounts(context.Background())
	require.NoError(t, err)
	assert.Len(t, mounts, 2)
	assert.Equal(t, "kv", mounts["secret/"].Type)
	assert.Equal(t, "1", mounts["secret/"].Options["version"])
	assert.Equal(t, "kv", mounts["kv2/"].Type)
	assert.Equal(t, "2", mounts["kv2/"].Options["version"])
}

func TestVaultError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer server.Close()

	c, err := NewClient(server.URL, nil)
	require.NoError(t, err)
	c.SetToken("bad-token")

	_, err = c.Read(context.Background(), "secret/foo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestTokenID(t *testing.T) {
	t.Run("nil secret", func(t *testing.T) {
		var s *Secret
		_, err := s.TokenID()
		assert.Error(t, err)
	})

	t.Run("nil auth", func(t *testing.T) {
		s := &Secret{}
		_, err := s.TokenID()
		assert.Error(t, err)
	})

	t.Run("valid", func(t *testing.T) {
		s := &Secret{Auth: &SecretAuth{ClientToken: "tok"}}
		tok, err := s.TokenID()
		require.NoError(t, err)
		assert.Equal(t, "tok", tok)
	})
}

func TestWriteEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty body (like KV v1 write).
	}))
	defer server.Close()

	c, err := NewClient(server.URL, nil)
	require.NoError(t, err)
	c.SetToken("test-token")

	secret, err := c.Write(context.Background(), "secret/foo", map[string]interface{}{"key": "val"})
	require.NoError(t, err)
	assert.NotNil(t, secret)
}
