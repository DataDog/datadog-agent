// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package secret contains the structure to receive and return secrets to the Datadog Agent
package secret

import "errors"

// Input represents a secret to be resolved
type Input struct {
	Secrets              []string               `json:"secrets"`
	Version              string                 `json:"version"`
	Type                 string                 `json:"type"`
	Config               map[string]interface{} `json:"config"`
	SecretBackendTimeout *float64               `json:"secret_backend_timeout,omitempty"`
}

// Output represents a resolved secret
type Output struct {
	Value *string `json:"value"`
	Error *string `json:"error"`
}

// ErrKeyNotFound is returned when the secret key is not found
var ErrKeyNotFound = errors.New("backend does not provide secret key")

const (
	// DefaultMaxFileReadSize is the maximum file size (10 MB) that can be read as a secret
	DefaultMaxFileReadSize = 10 * 1024 * 1024
)
