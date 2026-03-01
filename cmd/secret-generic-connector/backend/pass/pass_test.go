// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pass

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// mockPassBinary creates a script that mimics `pass show <key>` using plain files.
func mockPassBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "pass")
	require.NoError(t, os.WriteFile(bin, []byte(
		"#!/bin/sh\nf=\"$PASSWORD_STORE_DIR/$2\"\n"+
			"if [ -f \"$f\" ]; then cat \"$f\"; else exit 1; fi\n",
	), 0755))
	return bin
}

func TestGetSecretWithPrefix(t *testing.T) {
	store := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(store, "datadog"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(store, "datadog", "api_key"), []byte("my-api-key\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(store, "datadog", "app_key"), []byte("my-app-key\n"), 0644))

	b := &Backend{Config: BackendConfig{
		PassBinary: mockPassBinary(t),
		StorePath:  store,
		Prefix:     "datadog/",
	}}
	ctx := context.Background()

	out := b.GetSecretOutput(ctx, "api_key")
	assert.Nil(t, out.Error)
	assert.Equal(t, "my-api-key", *out.Value)

	out = b.GetSecretOutput(ctx, "app_key")
	assert.Nil(t, out.Error)
	assert.Equal(t, "my-app-key", *out.Value)
}

func TestGetSecretWithoutPrefix(t *testing.T) {
	store := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(store, "api_key"), []byte("plain-key\n"), 0644))

	b := &Backend{Config: BackendConfig{
		PassBinary: mockPassBinary(t),
		StorePath:  store,
	}}

	out := b.GetSecretOutput(context.Background(), "api_key")
	assert.Nil(t, out.Error)
	assert.Equal(t, "plain-key", *out.Value)
}

func TestGetSecretNotFound(t *testing.T) {
	b := &Backend{Config: BackendConfig{
		PassBinary: mockPassBinary(t),
		StorePath:  t.TempDir(),
		Prefix:     "datadog/",
	}}

	out := b.GetSecretOutput(context.Background(), "nonexistent")
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
	assert.Contains(t, *out.Error, "pass lookup failed")
}

func TestGetSecretEmptyValue(t *testing.T) {
	store := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(store, "empty"), []byte(""), 0644))

	b := &Backend{Config: BackendConfig{
		PassBinary: mockPassBinary(t),
		StorePath:  store,
	}}

	out := b.GetSecretOutput(context.Background(), "empty")
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *out.Error)
}
