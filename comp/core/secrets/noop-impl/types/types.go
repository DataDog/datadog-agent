// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides the types for the noop implementation of the secrets component
package types

import secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"

// SecretNoop is a noop implementation of the secrets component
type SecretNoop struct{}

var _ secrets.Component = (*SecretNoop)(nil)

// Configure does nothing
func (r *SecretNoop) Configure(_ secrets.ConfigParams) {}

// SubscribeToChanges does nothing
func (r *SecretNoop) SubscribeToChanges(_ secrets.SecretChangeCallback) {}

// Resolve does nothing
func (r *SecretNoop) Resolve(data []byte, _ string, _ string, _ string, _ bool) ([]byte, error) {
	return data, nil
}

// Refresh does nothing
func (r *SecretNoop) Refresh(_ bool) (string, error) {
	return "", nil
}

// RemoveOrigin
func (r *SecretNoop) RemoveOrigin(_ string) {}
