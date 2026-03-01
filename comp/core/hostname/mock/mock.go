// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the hostname component.
package mock

import (
	"context"
	"testing"

	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
)

type mockHostname struct {
	name string
}

func (m *mockHostname) Get(_ context.Context) (string, error) {
	return m.name, nil
}

func (m *mockHostname) GetWithProvider(_ context.Context) (hostnamedef.Data, error) {
	return hostnamedef.Data{Hostname: m.name, Provider: "mock"}, nil
}

func (m *mockHostname) GetSafe(_ context.Context) string {
	return m.name
}

// New returns a mock hostname component that always returns the given hostname.
func New(_ *testing.T, hostname string) hostnamedef.Component {
	return &mockHostname{name: hostname}
}
