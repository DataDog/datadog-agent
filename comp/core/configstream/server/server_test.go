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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

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

func setupTest(ctx context.Context, t *testing.T) (*Server, *mockComp, *mockStream, chan *pb.ConfigEvent) {
	cfg := configmock.New(t)
	cfg.Set("config_stream.sleep_interval", 10*time.Millisecond, model.SourceAgentRuntime)

	comp := &mockComp{}
	stream := &mockStream{ctx: ctx}

	server := NewServer(cfg, comp)
	eventsCh := make(chan *pb.ConfigEvent, 1)

	return server, comp, stream, eventsCh
}

func TestStreamConfigEventsErrors(t *testing.T) {
	testReq := &pb.ConfigStreamRequest{}
	testEvent := &pb.ConfigEvent{}

	t.Run("returns error on terminal error from stream Send", func(t *testing.T) {
		server, comp, stream, eventsCh := setupTest(context.Background(), t)
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

		server, comp, stream, eventsCh := setupTest(ctx, t)
		var unsubscribeCalled bool
		unsubscribe := func() { unsubscribeCalled = true }
		comp.On("Subscribe", testReq).Return((<-chan *pb.ConfigEvent)(eventsCh), unsubscribe).Once()

		nonTerminalError := errors.New("some other error")
		firstSendSignal := make(chan struct{})
		secondSendSignal := make(chan struct{})

		// First call fails and sends a signal that it has been processed
		stream.On("Send", testEvent).Return(nonTerminalError).Run(func(_ mock.Arguments) {
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
