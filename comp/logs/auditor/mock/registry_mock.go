// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the auditor component
package mock

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// RegistryMock does nothing
type RegistryMock struct {
	offset      string
	tailingMode string
}

// GetOffset returns the offset.
func (r *RegistryMock) GetOffset(_ string) string {
	return r.offset
}

// SetOffset sets the offset.
func (r *RegistryMock) SetOffset(offset string) {
	r.offset = offset
}

// GetTailingMode returns the tailing mode.
func (r *RegistryMock) GetTailingMode(_ string) string {
	return r.tailingMode
}

// SetTailingMode sets the tailing mode.
func (r *RegistryMock) SetTailingMode(tailingMode string) {
	r.tailingMode = tailingMode
}

// Channel returns a channel
func (r *RegistryMock) Channel() chan *message.Payload {
	return nil
}

// Start does nothing
func (r *RegistryMock) Start() {}

// Stop does nothing
func (r *RegistryMock) Stop() {}

// Mock returns a mock for auditor component.
func Mock() *RegistryMock {
	return &RegistryMock{}
}
