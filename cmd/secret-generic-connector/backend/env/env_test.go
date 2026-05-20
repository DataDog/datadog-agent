// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package env

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

func TestNewEnvBackendNilConfig(t *testing.T) {
	backend, err := NewEnvBackend(nil)
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Nil(t, backend.allowed)
}

func TestNewEnvBackendEmptyAllowedKeys(t *testing.T) {
	backend, err := NewEnvBackend(map[string]interface{}{"allowed_keys": []string{}})
	assert.NoError(t, err)
	assert.Nil(t, backend.allowed)
}

func TestNewEnvBackendAllowedKeys(t *testing.T) {
	backend, err := NewEnvBackend(map[string]interface{}{"allowed_keys": []string{"FOO", "BAR"}})
	assert.NoError(t, err)
	assert.Len(t, backend.allowed, 2)
	assert.Contains(t, backend.allowed, "FOO")
	assert.Contains(t, backend.allowed, "BAR")
}

func TestNewEnvBackendInvalidConfigType(t *testing.T) {
	// allowed_keys must decode to []string; a string scalar fails decoding.
	backend, err := NewEnvBackend(map[string]interface{}{"allowed_keys": 42})
	assert.Nil(t, backend)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to map backend configuration")
}

func TestGetSecretOutputNoAllowlistReturnsValue(t *testing.T) {
	t.Setenv("TEST_ENV_BACKEND_VALUE", "hello")
	backend, err := NewEnvBackend(nil)
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_VALUE")
	assert.Nil(t, out.Error)
	if assert.NotNil(t, out.Value) {
		assert.Equal(t, "hello", *out.Value)
	}
}

func TestGetSecretOutputUnsetReturnsNotFound(t *testing.T) {
	backend, err := NewEnvBackend(nil)
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_DOES_NOT_EXIST")
	assert.Nil(t, out.Value)
	if assert.NotNil(t, out.Error) {
		assert.Equal(t, secret.ErrKeyNotFound.Error(), *out.Error)
	}
}

// An exported-but-empty env var is indistinguishable from unset via
// os.Getenv, so it should map to ErrKeyNotFound.
func TestGetSecretOutputEmptyValueReturnsNotFound(t *testing.T) {
	t.Setenv("TEST_ENV_BACKEND_EMPTY", "")
	backend, err := NewEnvBackend(nil)
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_EMPTY")
	assert.Nil(t, out.Value)
	if assert.NotNil(t, out.Error) {
		assert.Equal(t, secret.ErrKeyNotFound.Error(), *out.Error)
	}
}

func TestGetSecretOutputAllowlistPermits(t *testing.T) {
	t.Setenv("TEST_ENV_BACKEND_ALLOWED", "ok")
	backend, err := NewEnvBackend(map[string]interface{}{
		"allowed_keys": []string{"TEST_ENV_BACKEND_ALLOWED"},
	})
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_ALLOWED")
	assert.Nil(t, out.Error)
	if assert.NotNil(t, out.Value) {
		assert.Equal(t, "ok", *out.Value)
	}
}

func TestGetSecretOutputAllowlistDenies(t *testing.T) {
	t.Setenv("TEST_ENV_BACKEND_DENIED", "secret")
	backend, err := NewEnvBackend(map[string]interface{}{
		"allowed_keys": []string{"SOMETHING_ELSE"},
	})
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_DENIED")
	assert.Nil(t, out.Value)
	if assert.NotNil(t, out.Error) {
		assert.Contains(t, *out.Error, "not in allowed_keys")
	}
}

// Even when a key is on the allowlist, an unset var still returns
// ErrKeyNotFound rather than leaking the allowlist check error.
func TestGetSecretOutputAllowlistUnsetReturnsNotFound(t *testing.T) {
	backend, err := NewEnvBackend(map[string]interface{}{
		"allowed_keys": []string{"TEST_ENV_BACKEND_ALLOWED_BUT_UNSET"},
	})
	assert.NoError(t, err)

	out := backend.GetSecretOutput(context.Background(), "TEST_ENV_BACKEND_ALLOWED_BUT_UNSET")
	assert.Nil(t, out.Value)
	if assert.NotNil(t, out.Error) {
		assert.Equal(t, secret.ErrKeyNotFound.Error(), *out.Error)
	}
}
