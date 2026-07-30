// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// fakeRemoteAgentRegistry is a minimal remoteagentregistry.Component used to exercise the gRPC handlers in isolation.
type fakeRemoteAgentRegistry struct {
	reportErr    error
	gotSessionID string
	gotEvents    []remoteagentregistry.RemoteAgentEvent
}

func (f *fakeRemoteAgentRegistry) RegisterRemoteAgent(*remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "", 0, nil
}
func (f *fakeRemoteAgentRegistry) RefreshRemoteAgent(string) bool { return true }
func (f *fakeRemoteAgentRegistry) ReportRemoteAgentEvent(sessionID string, events []remoteagentregistry.RemoteAgentEvent) error {
	f.gotSessionID = sessionID
	f.gotEvents = events
	return f.reportErr
}
func (f *fakeRemoteAgentRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent {
	return nil
}
func (f *fakeRemoteAgentRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	return nil
}

func TestReportRemoteAgentEventHandler(t *testing.T) {
	t.Run("converts events and returns ok for known session", func(t *testing.T) {
		fake := &fakeRemoteAgentRegistry{}
		srv := &remoteAgentServer{remoteAgentRegistry: fake}

		_, err := srv.ReportRemoteAgentEvent(context.Background(), &pb.ReportRemoteAgentEventRequest{
			SessionId: "session-123",
			Events: []*pb.Event{
				{
					Message: "invalid API key detected",
					Details: &pb.Event_InvalidApiKey{InvalidApiKey: &pb.InvalidApiKeyEvent{}},
				},
				{Message: "event with no details"},
			},
		})
		require.NoError(t, err)

		assert.Equal(t, "session-123", fake.gotSessionID)
		require.Len(t, fake.gotEvents, 2)
		require.IsType(t, &remoteagentregistry.InvalidAPIKey{}, fake.gotEvents[0].Details)
		assert.Equal(t, "invalid_api_key", fake.gotEvents[0].Details.EventType())
		assert.Equal(t, "invalid API key detected", fake.gotEvents[0].Message)
		assert.Nil(t, fake.gotEvents[1].Details)
	})

	t.Run("returns NotFound when the registry reports an error", func(t *testing.T) {
		fake := &fakeRemoteAgentRegistry{reportErr: errors.New("no remote agent found with session ID")}
		srv := &remoteAgentServer{remoteAgentRegistry: fake}

		_, err := srv.ReportRemoteAgentEvent(context.Background(), &pb.ReportRemoteAgentEventRequest{SessionId: "missing"})
		require.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("returns Unimplemented when the registry is not enabled", func(t *testing.T) {
		srv := &remoteAgentServer{remoteAgentRegistry: nil}

		_, err := srv.ReportRemoteAgentEvent(context.Background(), &pb.ReportRemoteAgentEventRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	})
}
