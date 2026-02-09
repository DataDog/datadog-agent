// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// mockStream is a mock of the AgentSecure_StreamConfigEventsServer interface
type mockStream struct {
	mock.Mock
	ctx context.Context
}

func (m *mockStream) Send(resp *pb.ConfigEvent) error {
	args := m.Called(resp)
	return args.Error(0)
}

func (m *mockStream) Context() context.Context {
	return m.ctx
}

func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(interface{}) error    { return nil }
func (m *mockStream) RecvMsg(interface{}) error    { return nil }

// mockComp is a mock of the configstream.Component interface
type mockComp struct {
	mock.Mock
}

func (m *mockComp) Subscribe(req *pb.ConfigStreamRequest) (<-chan *pb.ConfigEvent, func()) {
	args := m.Called(req)
	return args.Get(0).(<-chan *pb.ConfigEvent), args.Get(1).(func())
}

// mockRemoteAgentRegistry is a mock of the remoteagentregistry.Component interface
type mockRemoteAgentRegistry struct {
	mock.Mock
}

func (m *mockRemoteAgentRegistry) RegisterRemoteAgent(_ *remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "test-session-id", 30, nil
}

func (m *mockRemoteAgentRegistry) RefreshRemoteAgent(_ string) bool {
	// Always return true for tests (agent is registered)
	return true
}

func (m *mockRemoteAgentRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent {
	return nil
}

func (m *mockRemoteAgentRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	return nil
}

func setupTest(ctx context.Context, t *testing.T, sessionID string) (*Server, *mockComp, *mockStream, chan *pb.ConfigEvent) {
	cfg := configmock.New(t)
	cfg.Set("config_stream.sleep_interval", 10*time.Millisecond, model.SourceAgentRuntime)

	comp := &mockComp{}

	md := metadata.New(map[string]string{"session_id": sessionID})
	ctxWithMetadata := metadata.NewIncomingContext(ctx, md)
	stream := &mockStream{ctx: ctxWithMetadata}

	mockRAR := &mockRemoteAgentRegistry{}

	server := NewServer(cfg, comp, mockRAR)
	eventsCh := make(chan *pb.ConfigEvent, 1)

	return server, comp, stream, eventsCh
}

func TestStreamConfigEventsErrors(t *testing.T) {
	testReq := &pb.ConfigStreamRequest{
		Name: "test-client",
	}
	testEvent := &pb.ConfigEvent{}

	t.Run("returns error on terminal error from stream Send", func(t *testing.T) {
		server, comp, stream, eventsCh := setupTest(context.Background(), t, "test-session-id")
		unsubscribe := func() {}
		comp.On("Subscribe", testReq).Return((<-chan *pb.ConfigEvent)(eventsCh), unsubscribe).Once()

		terminalError := status.Error(codes.Unavailable, "network is down")
		stream.On("Send", testEvent).Return(terminalError).Once()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := server.StreamConfigEvents(testReq, stream)
			assert.Error(t, err)
			assert.Equal(t, terminalError, err)
		}()

		eventsCh <- testEvent
		wg.Wait()

		comp.AssertExpectations(t)
		stream.AssertExpectations(t)
	})

	t.Run("continues on non-terminal error from stream Send", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server, comp, stream, eventsCh := setupTest(ctx, t, "test-session-id")
		var unsubscribeCalled bool
		unsubscribe := func() { unsubscribeCalled = true }
		comp.On("Subscribe", testReq).Return((<-chan *pb.ConfigEvent)(eventsCh), unsubscribe).Once()

		errNonTerminal := errors.New("some other error")
		firstSendSignal := make(chan struct{})
		secondSendSignal := make(chan struct{})

		// First call fails and sends a signal that it has been processed
		stream.On("Send", testEvent).Return(errNonTerminal).Run(func(_ mock.Arguments) {
			firstSendSignal <- struct{}{}
		}).Once()

		// Second call succeeds and also sends a signal
		stream.On("Send", testEvent).Return(nil).Run(func(_ mock.Arguments) {
			secondSendSignal <- struct{}{}
		}).Once()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := server.StreamConfigEvents(testReq, stream)
			// Expect a context canceled error because we cancel it to end the test
			assert.ErrorIs(t, err, context.Canceled)
		}()

		// First event fails with non-terminal error
		eventsCh <- testEvent
		<-firstSendSignal

		// Second event succeeds
		eventsCh <- testEvent
		<-secondSendSignal

		cancel() // End the stream
		wg.Wait()

		comp.AssertExpectations(t)
		stream.AssertExpectations(t)
		assert.True(t, unsubscribeCalled, "unsubscribe should have been called")
	})

}

func TestRARAuthorization(t *testing.T) {
	testReq := &pb.ConfigStreamRequest{
		Name: "test-client",
	}

	t.Run("rejects request with missing metadata", func(t *testing.T) {
		cfg := configmock.New(t)
		comp := &mockComp{}
		mockRAR := &mockRemoteAgentRegistry{}
		server := NewServer(cfg, comp, mockRAR)

		// Context without metadata
		stream := &mockStream{ctx: context.Background()}

		err := server.StreamConfigEvents(testReq, stream)
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		require.ErrorContains(t, err, "missing gRPC metadata")
	})

	t.Run("rejects request with missing session_id in metadata", func(t *testing.T) {
		cfg := configmock.New(t)
		comp := &mockComp{}
		mockRAR := &mockRemoteAgentRegistry{}
		server := NewServer(cfg, comp, mockRAR)

		// Context with metadata but no session_id
		md := metadata.New(map[string]string{})
		ctx := metadata.NewIncomingContext(context.Background(), md)
		stream := &mockStream{ctx: ctx}

		err := server.StreamConfigEvents(testReq, stream)
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		require.ErrorContains(t, err, "session_id required in metadata")
	})

	t.Run("rejects request with empty session_id", func(t *testing.T) {
		cfg := configmock.New(t)
		comp := &mockComp{}
		mockRAR := &mockRemoteAgentRegistry{}
		server := NewServer(cfg, comp, mockRAR)

		// Context with empty session_id
		md := metadata.New(map[string]string{"session_id": ""})
		ctx := metadata.NewIncomingContext(context.Background(), md)
		stream := &mockStream{ctx: ctx}

		err := server.StreamConfigEvents(testReq, stream)
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		require.ErrorContains(t, err, "session_id cannot be empty")
	})
}
