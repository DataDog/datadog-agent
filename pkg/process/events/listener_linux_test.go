// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package events

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api/mocks"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
)

// TestProcessEventFiltering asserts that sysProbeListener collects only expected events and drops everything else
func TestProcessEventFiltering(t *testing.T) {
	rawEvents := make([]*model.ProcessEvent, 0)
	handlers := make([]EventHandler, 0)

	// The listener should drop unexpected events and not call the EventHandler for it
	rawEvents = append(rawEvents, model.NewMockedForkEvent(time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}))

	// Verify that expected events are correctly consumed
	rawEvents = append(rawEvents, model.NewMockedExecEvent(time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}))
	handlers = append(handlers, func(e *model.ProcessEvent) {
		require.Equal(t, model.Exec, e.EventType)
		require.Equal(t, uint32(23), e.Pid)
	})

	rawEvents = append(rawEvents, model.NewMockedExitEvent(time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}, 0))
	handlers = append(handlers, func(e *model.ProcessEvent) {
		require.Equal(t, model.Exit, e.EventType)
		require.Equal(t, uint32(23), e.Pid)
	})

	// To avoid race conditions, all handlers should be assigned during the creation of SysProbeListener
	calledHandlers := 0
	handler := func(e *model.ProcessEvent) {
		handlers[calledHandlers](e)
		calledHandlers++
	}

	l, err := NewSysProbeListener(nil, nil, handler)
	require.NoError(t, err)

	for _, e := range rawEvents {
		data, err := e.MarshalMsg(nil)
		require.NoError(t, err)
		l.consumeData(data)
	}
	assert.Equal(t, len(handlers), calledHandlers)
}

// TestProcessEventHandling mocks a SecurityModuleClient and asserts that the Listener bubbles up the correct events
func TestProcessEventHandling(t *testing.T) {
	ctx := context.Background()

	client := mocks.NewEventMonitoringModuleClient(t)
	stream := mocks.NewEventMonitoringModule_GetProcessEventsClient(t)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{TimeoutSeconds: 1}).Return(stream, nil)

	events := make([]*model.ProcessEvent, 0)
	events = append(events, model.NewMockedExecEvent(time.Now().Add(-10*time.Second), 32, "/usr/bin/ls", []string{"ls", "-lah"}))
	events = append(events, model.NewMockedExitEvent(time.Now().Add(-9*time.Second), 32, "/usr/bin/ls", []string{"ls", "-lah"}, 0))
	events = append(events, model.NewMockedExecEvent(time.Now().Add(-5*time.Second), 32, "/usr/bin/ls", []string{"ls", "invalid-path"}))
	events = append(events, model.NewMockedExitEvent(time.Now().Add(-5*time.Second), 32, "/usr/bin/ls", []string{"ls", "invalid-path"}, 2))

	for _, e := range events {
		data, err := e.MarshalMsg(nil)
		require.NoError(t, err)

		stream.On("Recv").Once().Return(&api.ProcessEventMessage{Data: data}, nil)
	}
	stream.On("Recv").Return(nil, io.EOF)

	rcvMessage := make(chan bool)
	i := 0
	handler := func(e *model.ProcessEvent) {
		if i > len(events)-1 {
			t.Error("should not have received more process events")
		}

		AssertProcessEvents(t, events[i], e)
		// all message have been consumed
		if i == len(events)-1 {
			close(rcvMessage)
		}

		i++
	}
	l, err := NewSysProbeListener(nil, client, handler)
	require.NoError(t, err)
	l.Run()

	<-rcvMessage
	l.Stop()
	client.AssertExpectations(t)
	stream.AssertExpectations(t)
}

// TestSecurityModuleClientReconnect asserts that process-agent is able to reconnect to system-probe if the connection
// is dropped
func TestSecurityModuleClientReconnect(t *testing.T) {
	ctx := context.Background()

	client := mocks.NewEventMonitoringModuleClient(t)
	stream := mocks.NewEventMonitoringModule_GetProcessEventsClient(t)

	l, err := NewSysProbeListener(nil, client, func(e *model.ProcessEvent) {})
	require.NoError(t, err)

	l.retryInterval = 10 * time.Millisecond // force a fast retry for tests
	require.NoError(t, err)

	// Simulate that the event listener starts connected to the SecurityModule server
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{TimeoutSeconds: 1}).Return(stream, nil).Once()
	stream.On("Recv").Return(nil, io.EOF)

	// Then disconnects from it
	drop := make(chan time.Time)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{TimeoutSeconds: 1}).Return(stream, errors.New("server not available")).WaitUntil(drop).Once()

	// And reconnects
	reconnect := make(chan time.Time)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{TimeoutSeconds: 1}).Return(stream, nil).WaitUntil(reconnect)

	l.Run()
	assert.Eventually(t, func() bool { return l.connected.Load() == true }, 2*time.Second, 20*time.Millisecond,
		"event listener can't connect to SecurityModule server")

	// Next call to mocked GetProcessEvents blocks until drop channel is closed
	close(drop)
	assert.Eventually(t, func() bool { return l.connected.Load() == false }, 2*time.Second, 20*time.Millisecond,
		"event listener shouldn't be connected to SecurityModule server")

	// Next call to mocked GetProcessEvents blocks until reconnect channel is closed
	close(reconnect)
	assert.Eventually(t, func() bool { return l.connected.Load() == true }, 2*time.Second, 20*time.Millisecond,
		"event listener should be connected to SecurityModule server")

	l.Stop()

	client.AssertExpectations(t)
	stream.AssertExpectations(t)
}
