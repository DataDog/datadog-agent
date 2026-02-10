// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fileimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func setupTest(t *testing.T) (identitystore.Component, config.Component, func()) {
	// Create temp directory
	tempDir := t.TempDir()

	// Create mock config
	cfg := mock.New(t)
	cfg.SetWithoutSource(parIdentityFilePath, filepath.Join(tempDir, "test_identity.json"))

	// Create mock logger
	logger := logmock.New(t)

	// Create file store
	reqs := Requires{
		Config: cfg,
		Log:    logger,
	}
	provides := NewComponent(reqs)

	cleanup := func() {
		// Cleanup is handled by t.TempDir()
	}

	return provides.Comp, cfg, cleanup
}

func TestFileStore_PersistAndGetIdentity(t *testing.T) {
	store, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-private-key-base64",
		URN:        "urn:dd:apps:on-prem-runner:us:123456789:runner-id-xyz",
	}

	// Persist identity
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Retrieve identity
	retrievedIdentity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedIdentity)

	// Verify identity matches
	assert.Equal(t, testIdentity.PrivateKey, retrievedIdentity.PrivateKey)
	assert.Equal(t, testIdentity.URN, retrievedIdentity.URN)
}

func TestFileStore_GetIdentity_NotExists(t *testing.T) {
	store, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get identity that doesn't exist
	identity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.Nil(t, identity)
}

func TestFileStore_GetIdentity_InvalidJSON(t *testing.T) {
	store, cfg, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Write invalid JSON to file
	filePath := cfg.GetString(parIdentityFilePath)
	err := os.WriteFile(filePath, []byte("invalid json"), 0600)
	require.NoError(t, err)

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "failed to parse identity file JSON")
}

func TestFileStore_GetIdentity_EmptyURN(t *testing.T) {
	store, cfg, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Manually write empty URN to test validation
	filePath := cfg.GetString(parIdentityFilePath)
	err := os.WriteFile(filePath, []byte(`{"private_key":"test-key","urn":""}`), 0600)
	require.NoError(t, err)

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "URN is empty")
}

func TestFileStore_GetIdentity_EmptyPrivateKey(t *testing.T) {
	store, cfg, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Manually write empty private key to test validation
	filePath := cfg.GetString(parIdentityFilePath)
	err := os.WriteFile(filePath, []byte(`{"private_key":"","urn":"urn:dd:apps:on-prem-runner:us:123:456"}`), 0600)
	require.NoError(t, err)

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "private key is empty")
}

func TestFileStore_DeleteIdentity(t *testing.T) {
	store, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create and persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-private-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Verify identity exists
	identity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.NotNil(t, identity)

	// Delete identity
	err = store.DeleteIdentity(ctx)
	require.NoError(t, err)

	// Verify identity is deleted
	identity, err = store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.Nil(t, identity)
}

func TestFileStore_DeleteIdentity_NotExists(t *testing.T) {
	store, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Delete identity that doesn't exist (should not error)
	err := store.DeleteIdentity(ctx)
	assert.NoError(t, err)
}

func TestFileStore_UpdateIdentity(t *testing.T) {
	store, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create and persist initial identity
	initialIdentity := &identitystore.Identity{
		PrivateKey: "initial-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, initialIdentity)
	require.NoError(t, err)

	// Update with new identity
	updatedIdentity := &identitystore.Identity{
		PrivateKey: "updated-key",
		URN:        "urn:dd:apps:on-prem-runner:us:789:012",
	}
	err = store.PersistIdentity(ctx, updatedIdentity)
	require.NoError(t, err)

	// Retrieve and verify updated identity
	retrievedIdentity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedIdentity)

	assert.Equal(t, updatedIdentity.PrivateKey, retrievedIdentity.PrivateKey)
	assert.Equal(t, updatedIdentity.URN, retrievedIdentity.URN)
	assert.NotEqual(t, initialIdentity.PrivateKey, retrievedIdentity.PrivateKey)
}

func TestFileStore_FilePathPriority(t *testing.T) {
	tests := []struct {
		name             string
		configPath       string
		authTokenPath    string
		expectedFileName string
	}{
		{
			name:             "explicit config path takes priority",
			configPath:       "/custom/path/identity.json",
			authTokenPath:    "/some/auth/path/auth_token",
			expectedFileName: "/custom/path/identity.json",
		},
		{
			name:             "auth token path fallback",
			configPath:       "",
			authTokenPath:    "/auth/path/auth_token",
			expectedFileName: "/auth/path/privateactionrunner_private_identity.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mock.New(t)
			logger := logmock.New(t)

			if tt.configPath != "" {
				cfg.SetWithoutSource(parIdentityFilePath, tt.configPath)
			}
			if tt.authTokenPath != "" {
				cfg.SetWithoutSource("auth_token_file_path", tt.authTokenPath)
			}

			fs := &fileStore{
				config: cfg,
				log:    logger,
			}

			actualPath := fs.getIdentityFilePath()
			assert.Equal(t, tt.expectedFileName, actualPath)
		})
	}
}

func TestFileStore_FilePermissions(t *testing.T) {
	store, cfg, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create and persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Check file permissions
	filePath := cfg.GetString(parIdentityFilePath)
	fileInfo, err := os.Stat(filePath)
	require.NoError(t, err)

	// Verify permissions are 0600
	assert.Equal(t, os.FileMode(0600), fileInfo.Mode().Perm())
}
