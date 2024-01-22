// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package testutils provides util test setup methods for pkg/logs
package testutils

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
)

type mockHostnameService struct {
	name string
}

func (m *mockHostnameService) Get(_ context.Context) (string, error) {
	return m.name, nil
}

func (m *mockHostnameService) GetSafe(_ context.Context) string {
	return m.name
}

func (m *mockHostnameService) Set(name string) {
	m.name = name
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
func (m *mockHostnameService) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: m.name,
		Provider: "mockService",
	}, nil
}

// NewHostnameMock returns a mock instance of hostnameinterface to be used in tests
func NewHostnameMock(name string) hostnameinterface.HostnameInterface {
	return &mockHostnameService{name}
}
