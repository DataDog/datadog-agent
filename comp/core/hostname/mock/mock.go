// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the hostname component.
package mock

import (
	"context"
	"testing"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/def"
)

// Mock implements mock-specific methods.
type Mock interface {
	hostname.Component
	// Set overrides the hostname returned by the mock.
	Set(name string)
}

type mockService struct {
	name string
}

var _ Mock = (*mockService)(nil)

func (m *mockService) Get(_ context.Context) (string, error) {
	return m.name, nil
}

func (m *mockService) GetSafe(_ context.Context) string {
	return m.name
}

func (m *mockService) Set(name string) {
	m.name = name
}

func (m *mockService) GetWithProvider(_ context.Context) (hostname.Data, error) {
	return hostname.Data{
		Hostname: m.name,
		Provider: "mockService",
	}, nil
}

// New returns a Component mock for use in tests that don't need the full fx wiring.
func New(t *testing.T) Mock {
	_ = t // reserved for future cleanup hooks
	return &mockService{name: "my-hostname"}
}
