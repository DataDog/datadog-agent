// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows

// Package windows provides a stub for the Windows registry key secret backend on non-Windows platforms.
package windows

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// WindowsRegkeyBackend represents backend for WindowsRegkey
type WindowsRegkeyBackend struct {
}

// NewWindowsRegkeyBackend returns a new WindowsRegkey backend
func NewWindowsRegkeyBackend(_ map[string]interface{}) (*WindowsRegkeyBackend, error) {
	backend := &WindowsRegkeyBackend{}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *WindowsRegkeyBackend) GetSecretOutput(_ context.Context, secretKey string) secret.Output {
	es := fmt.Sprintf("Error fetching secret '%s': windows.regkey is only supported on windows", secretKey)
	return secret.Output{Value: nil, Error: &es}
}
