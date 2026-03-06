// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pass allows fetching secrets from the pass password manager (https://www.passwordstore.org/)
package pass

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// BackendConfig is the configuration for the pass backend
type BackendConfig struct {
	// StorePath overrides PASSWORD_STORE_DIR (default: ~/.password-store)
	StorePath string `mapstructure:"store_path"`
	// Prefix is prepended to all secret lookups
	Prefix string `mapstructure:"prefix"`
	// PassBinary is the path to the pass executable (default: "pass" from PATH)
	PassBinary string `mapstructure:"pass_binary"`
}

// Backend represents the pass password manager backend
type Backend struct {
	Config BackendConfig
}

// NewBackend returns a new pass backend
func NewBackend(bc map[string]interface{}) (*Backend, error) {
	cfg := BackendConfig{}
	if err := mapstructure.Decode(bc, &cfg); err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	if cfg.PassBinary == "" {
		cfg.PassBinary = "pass"
	}

	if _, err := exec.LookPath(cfg.PassBinary); err != nil {
		return nil, fmt.Errorf("'%s' executable not found in PATH: %w", cfg.PassBinary, err)
	}

	return &Backend{Config: cfg}, nil
}

// GetSecretOutput retrieves a secret from the pass store
func (b *Backend) GetSecretOutput(ctx context.Context, secretKey string) secret.Output {
	path := b.Config.Prefix + secretKey

	cmd := exec.CommandContext(ctx, b.Config.PassBinary, "show", path)
	if b.Config.StorePath != "" {
		cmd.Env = append(cmd.Environ(), "PASSWORD_STORE_DIR="+b.Config.StorePath)
	}

	out, err := cmd.Output()
	if err != nil {
		es := fmt.Sprintf("pass lookup failed for '%s': %s", path, err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	// pass outputs the secret followed by a newline
	value := strings.TrimRight(string(out), "\n")
	if value == "" {
		es := secret.ErrKeyNotFound.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &value, Error: nil}
}
