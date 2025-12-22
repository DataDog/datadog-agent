// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-secret-backend/secret"
)

func TestFileBackendGetSecretOutput(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "api_key"), []byte("api-key"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "app_key"), []byte("app-key"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "empty"), []byte(""), 0644)
	assert.NoError(t, err)

	backend := &TextFileBackend{
		Config: TextFileBackendConfig{
			SecretsPath:     tmpDir,
			MaxFileReadSize: secret.DefaultMaxFileReadSize,
		},
	}

	ctx := context.Background()

	tests := []struct {
		name   string
		secret string
		value  string
		fail   bool
	}{
		{
			name:   "valid secret",
			secret: "api_key",
			value:  "api-key",
			fail:   false,
		},
		{
			name:   "other secret",
			secret: "app_key",
			value:  "app-key",
			fail:   false,
		},
		{
			name:   "secret not found",
			secret: "nonexistent",
			fail:   true,
		},
		{
			name:   "empty secret name",
			secret: "",
			fail:   true,
		},
		{
			name:   "path traversal blocked",
			secret: "../etc/passwd",
			fail:   true,
		},
		{
			name:   "absolute path works",
			secret: filepath.Join(tmpDir, "api_key"),
			value:  "api-key",
			fail:   false,
		},
		{
			name:   "empty file fails",
			secret: "empty",
			fail:   true,
		},
		{
			name:   "slash in relative name blocked",
			secret: "subdir/secret",
			fail:   true,
		},
		{
			name:   "backslash in relative name blocked",
			secret: `subdir\secret`,
			fail:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := backend.GetSecretOutput(ctx, tt.secret)

			if tt.fail {
				assert.Nil(t, output.Value)
				assert.NotNil(t, output.Error)
			} else {
				assert.NotNil(t, output.Value)
				assert.Nil(t, output.Error)
				assert.Equal(t, tt.value, *output.Value)
			}
		})
	}
}

func TestNewFileBackend(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name   string
		config map[string]interface{}
		fail   bool
	}{
		{
			name: "valid config with path",
			config: map[string]interface{}{
				"secrets_path": tmpDir,
			},
			fail: false,
		},
		{
			name:   "missing secrets_path fails",
			config: map[string]interface{}{},
			fail:   true,
		},
		{
			name: "nonexistent directory fails",
			config: map[string]interface{}{
				"secrets_path": "/nonexistent/path",
			},
			fail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewTextFileBackend(tt.config)

			if tt.fail {
				assert.Error(t, err)
				assert.Nil(t, backend)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, backend)
			}
		})
	}
}

func TestFileBackendMaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	smallFile := filepath.Join(tmpDir, "small_secret")
	err := os.WriteFile(smallFile, make([]byte, 100), 0644) // 100 bytes
	assert.NoError(t, err)

	largeFile := filepath.Join(tmpDir, "large_secret")
	err = os.WriteFile(largeFile, make([]byte, 15*1024*1024), 0644) // 15 MB
	assert.NoError(t, err)

	ctx := context.Background()

	t.Run("default max file size allows small files", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path": tmpDir,
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(secret.DefaultMaxFileReadSize), backend.Config.MaxFileReadSize)

		output := backend.GetSecretOutput(ctx, "small_secret")
		assert.NotNil(t, output.Value)
		assert.Nil(t, output.Error)
	})

	t.Run("default max file size rejects large files", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path": tmpDir,
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(secret.DefaultMaxFileReadSize), backend.Config.MaxFileReadSize)

		output := backend.GetSecretOutput(ctx, "large_secret")
		assert.Nil(t, output.Value)
		assert.NotNil(t, output.Error)
		assert.Contains(t, *output.Error, "exceeds maximum size limit")
	})

	t.Run("custom max file size rejects files exceeding limit", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path":       tmpDir,
			"max_file_read_size": 12 * 1024 * 1024, // 12 MB
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(12*1024*1024), backend.Config.MaxFileReadSize)

		output := backend.GetSecretOutput(ctx, "large_secret")
		assert.Nil(t, output.Value)
		assert.NotNil(t, output.Error)
		assert.Contains(t, *output.Error, "exceeds maximum size limit")
	})

	t.Run("custom max file size allows files within limit", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path":       tmpDir,
			"max_file_read_size": 12 * 1024 * 1024, // 12 MB
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(12*1024*1024), backend.Config.MaxFileReadSize)

		output := backend.GetSecretOutput(ctx, "small_secret")
		assert.NotNil(t, output.Value)
		assert.Nil(t, output.Error)
	})

	t.Run("zero max file size uses default", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path":       tmpDir,
			"max_file_read_size": 0,
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(secret.DefaultMaxFileReadSize), backend.Config.MaxFileReadSize)
	})

	t.Run("negative max file size uses default", func(t *testing.T) {
		backend, err := NewTextFileBackend(map[string]interface{}{
			"secrets_path":       tmpDir,
			"max_file_read_size": -1,
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(secret.DefaultMaxFileReadSize), backend.Config.MaxFileReadSize)
	})
}
