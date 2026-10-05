// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package windows provides a Windows registry key secret backend for the Datadog Agent.
package windows

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/sys/windows/registry"
)

// WindowsRegkeyBackendConfig is the configuration for a WindowsRegkey backend
type WindowsRegkeyBackendConfig struct {
	RootKeyStr string `mapstructure:"root_key"`
}

// WindowsRegkeyBackend represents backend for WindowsRegkey
type WindowsRegkeyBackend struct {
	Config  WindowsRegkeyBackendConfig
	RootKey registry.Key
}

// RootKeysMap maps user-facing registry hive names (short and long form,
// case-insensitive after upper-casing) to their registry.Key constants.
var RootKeysMap = map[string]registry.Key{
	"HKLM":                registry.LOCAL_MACHINE,
	"HKEY_LOCAL_MACHINE":  registry.LOCAL_MACHINE,
	"HKCU":                registry.CURRENT_USER,
	"HKEY_CURRENT_USER":   registry.CURRENT_USER,
	"HKCR":                registry.CLASSES_ROOT,
	"HKEY_CLASSES_ROOT":   registry.CLASSES_ROOT,
	"HKU":                 registry.USERS,
	"HKEY_USERS":          registry.USERS,
	"HKCC":                registry.CURRENT_CONFIG,
	"HKEY_CURRENT_CONFIG": registry.CURRENT_CONFIG,
}

// NewWindowsRegkeyBackend returns a new WindowsRegkey backend
func NewWindowsRegkeyBackend(bc map[string]interface{}) (*WindowsRegkeyBackend, error) {
	backendConfig := WindowsRegkeyBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	var ok bool
	var rootKey = registry.LOCAL_MACHINE
	if backendConfig.RootKeyStr != "" {
		rootKey, ok = RootKeysMap[strings.ToUpper(backendConfig.RootKeyStr)]
		if !ok {
			return nil, fmt.Errorf("unknown registry root %q", backendConfig.RootKeyStr)
		}
	}

	backend := &WindowsRegkeyBackend{
		Config:  backendConfig,
		RootKey: rootKey,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *WindowsRegkeyBackend) GetSecretOutput(_ context.Context, secretKey string) secret.Output {
	result := strings.SplitN(secretKey, ":", 2)
	if len(result) != 2 {
		es := fmt.Sprintf("Error fetching secret '%s': no delimeter found", secretKey)
		return secret.Output{Value: nil, Error: &es}
	}

	val, err := b.getRegKey(result[0], result[1])
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &val, Error: nil}
}

func (b *WindowsRegkeyBackend) getRegKey(path string, value string) (string, error) {
	// Check 64-bit path first
	k, err := registry.OpenKey(b.RootKey, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		// Fallback to 32-bit path just in case
		k, err = registry.OpenKey(b.RootKey, path, registry.QUERY_VALUE|registry.WOW64_32KEY)
		if err != nil {
			return "", err
		}
	}
	defer k.Close()

	s, _, err := k.GetStringValue(value)
	if err != nil {
		return "", err
	}

	return s, nil
}
