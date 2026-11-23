// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the remoteagentregistry component
package mock

import (
	"context"
	"testing"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Mock returns a mock for remoteagentregistry component.
func Mock(_ *testing.T) remoteagentregistry.Component {
	return &mockRegistry{}
}

type mockRegistry struct{}

func (m *mockRegistry) RegisterRemoteAgent(_ *remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "", 0, nil
}

func (m *mockRegistry) RefreshRemoteAgent(_ string) bool { return false }

func (m *mockRegistry) ReportRemoteAgentEvent(_ string, _ []remoteagentregistry.RemoteAgentEvent) error {
	return nil
}

func (m *mockRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent { return nil }

func (m *mockRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData { return nil }

func (m *mockRegistry) ListCommands(_ context.Context) []*pb.CommandProvider {
	return nil
}

func (m *mockRegistry) ExecuteCommand(_ context.Context, _ *pb.ExecuteCommandRequest, _ func(*pb.ExecuteCommandResponse) error) error {
	return nil
}
