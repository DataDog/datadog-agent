// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/stretchr/testify/assert"
)

func TestYAMLBackend(t *testing.T) {
	tmpDir := t.TempDir()
	secretsFilepath := filepath.Join(tmpDir, "secrets.yaml")
	tempFile, err := os.Create(secretsFilepath)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	secretsData := `
key1: value1
key2: value2
`
	if _, err := tempFile.Write([]byte(secretsData)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	yamlSecretsBackendParams := map[string]interface{}{
		"backend_type": "file.yaml",
		"file_path":    secretsFilepath,
	}
	yamlSecretsBackend, err := NewYAMLBackend("yaml-backend", yamlSecretsBackendParams)
	assert.NoError(t, err)

	assert.Equal(t, "yaml-backend", yamlSecretsBackend.BackendID)
	assert.Equal(t, "file.yaml", yamlSecretsBackend.Config.BackendType)
	assert.Equal(t, secretsFilepath, yamlSecretsBackend.Config.FilePath)

	secretOutput := yamlSecretsBackend.GetSecretOutput("key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = yamlSecretsBackend.GetSecretOutput("key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}
