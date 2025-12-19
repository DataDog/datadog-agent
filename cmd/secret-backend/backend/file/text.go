// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package file allows to fetch secrets from files
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// TextFileBackendConfig is the configuration for a file backend
type TextFileBackendConfig struct {
	SecretsPath     string `mapstructure:"secrets_path"`
	MaxFileReadSize int64  `mapstructure:"max_file_read_size"`
}

const (
	// DefaultMaxFileReadSize is the maximum file size (10 MB) that can be read as a secret
	DefaultMaxFileReadSize = 10 * 1024 * 1024
)

// TextFileBackend represents backend for individual secret files
type TextFileBackend struct {
	Config TextFileBackendConfig
}

// NewTextFileBackend returns a new file backend
func NewTextFileBackend(bc map[string]interface{}) (*TextFileBackend, error) {
	backendConfig := TextFileBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	if backendConfig.SecretsPath == "" {
		return nil, fmt.Errorf("secrets_path is required")
	}

	info, err := os.Stat(backendConfig.SecretsPath)
	if err != nil {
		return nil, fmt.Errorf("secrets path '%s' is not accessible: %w", backendConfig.SecretsPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("secrets path '%s' is not a directory", backendConfig.SecretsPath)
	}

	if backendConfig.MaxFileReadSize <= 0 {
		backendConfig.MaxFileReadSize = DefaultMaxFileReadSize
	}

	return &TextFileBackend{Config: backendConfig}, nil
}

// GetSecretOutput retrieves a secret from a file
func (b *TextFileBackend) GetSecretOutput(_ context.Context, secretString string) secret.Output {
	base := filepath.Clean(b.Config.SecretsPath)
	path := secretString
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	cleanPath := filepath.Clean(path)

	if !strings.HasPrefix(cleanPath, base+string(filepath.Separator)) && cleanPath != base {
		es := "path outside allowed directory"
		return secret.Output{Value: nil, Error: &es}
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		es := fmt.Sprintf("failed to stat secret file '%s': %s", secretString, err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	if info.Size() > b.Config.MaxFileReadSize {
		es := fmt.Sprintf("secret file '%s' exceeds maximum size limit of %d bytes (actual: %d bytes)", secretString, b.Config.MaxFileReadSize, info.Size())
		return secret.Output{Value: nil, Error: &es}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			es := secret.ErrKeyNotFound.Error()
			return secret.Output{Value: nil, Error: &es}
		}
		if os.IsPermission(err) {
			es := fmt.Sprintf("permission denied reading secret '%s'", secretString)
			return secret.Output{Value: nil, Error: &es}
		}
		es := fmt.Sprintf("failed to read secret '%s': %s", secretString, err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	value := string(data)
	if value == "" {
		es := secret.ErrKeyNotFound.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &value, Error: nil}
}
