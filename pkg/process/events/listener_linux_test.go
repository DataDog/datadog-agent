// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

//go:build linux
// +build linux

package events

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/api/mocks"
)

// assertProcessEvents compares two ProcessEvents. Two events can't be compared using directly assert.Equal
// due to the embedded time fields
func assertProcessEvents(t *testing.T, expected, actual *model.ProcessEvent) {
	t.Helper()

	assert.Equal(t, expected.EventType, actual.EventType)
	assert.WithinDuration(t, expected.CollectionTime, actual.CollectionTime, 0)
	assert.Equal(t, expected.Pid, actual.Pid)
	assert.Equal(t, expected.Ppid, actual.Ppid)
	assert.Equal(t, expected.UID, actual.UID)
	assert.Equal(t, expected.GID, actual.GID)
	assert.Equal(t, expected.Username, actual.Username)
	assert.Equal(t, expected.Group, actual.Group)
	assert.Equal(t, expected.Exe, actual.Exe)
	assert.Equal(t, expected.Cmdline, actual.Cmdline)
	assert.WithinDuration(t, expected.ForkTime, actual.ForkTime, 0)
	assert.WithinDuration(t, expected.ExecTime, actual.ExecTime, 0)
	assert.WithinDuration(t, expected.ExitTime, actual.ExitTime, 0)
}

// TestProcessEventFiltering asserts that sysProbeListener collects only expected events and drops everything else
func TestProcessEventFiltering(t *testing.T) {
	rawEvents := make([]*model.ProcessMonitoringEvent, 0)
	handlers := make([]EventHandler, 0)

	// The listener should drop unexpected events and not call the EventHandler for it
	rawEvents = append(rawEvents, model.NewProcessMonitoringEvent(model.Fork, time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}))

	// Verify that expected events are correctly consumed
	rawEvents = append(rawEvents, model.NewProcessMonitoringEvent(model.Exec, time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}))
	handlers = append(handlers, func(e *model.ProcessEvent) {
		require.Equal(t, model.Exec, e.EventType)
		require.Equal(t, uint32(23), e.Pid)
	})

	rawEvents = append(rawEvents, model.NewProcessMonitoringEvent(model.Exit, time.Now(), 23, "/usr/bin/ls", []string{"ls", "-lah"}))
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

	l, err := newSysProbeListener(nil, nil, handler)
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

	client := mocks.NewSecurityModuleClient(t)
	stream := mocks.NewSecurityModule_GetProcessEventsClient(t)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{}).Return(stream, nil)

	now := time.Now()
	events := make([]*model.ProcessEvent, 0)
	e1 := model.ProcessEvent{
		EventType:      model.Exec,
		CollectionTime: now,
		Pid:            32,
		Ppid:           1,
		UID:            124,
		GID:            2,
		Username:       "agent",
		Group:          "dd-agent",
		Exe:            "/usr/bin/ls",
		Cmdline:        []string{"ls", "-lah"},
		ExecTime:       time.Now().Add(-10 * time.Second),
	}
	events = append(events, &e1)

	e2 := e1
	e2.EventType = model.Exit
	e2.ExitTime = time.Now().Add(-2 * time.Second)
	events = append(events, &e2)

	for _, e := range events {
		sysEvent := model.ProcessEventToProcessMonitoringEvent(e)
		data, err := sysEvent.MarshalMsg(nil)
		require.NoError(t, err)

		stream.On("Recv").Once().Return(&api.SecurityProcessEventMessage{Data: data}, nil)
	}
	stream.On("Recv").Return(nil, io.EOF)

	rcvMessage := make(chan bool)
	i := 0
	handler := func(e *model.ProcessEvent) {
		if i > len(events)-1 {
			t.Error("should not have received more process events")
		}

		assertProcessEvents(t, events[i], e)
		// all message have been consumed
		if i == len(events)-1 {
			close(rcvMessage)
		}

		i++
	}
	l, err := newSysProbeListener(nil, client, handler)
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

	client := mocks.NewSecurityModuleClient(t)
	stream := mocks.NewSecurityModule_GetProcessEventsClient(t)

	l, err := newSysProbeListener(nil, client, func(e *model.ProcessEvent) { return })
	require.NoError(t, err)

	l.retryInterval = 10 * time.Millisecond // force a fast retry for tests
	require.NoError(t, err)

	// Simulate that the event listener starts connected to the SecurityModule server
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{}).Return(stream, nil).Once()
	stream.On("Recv").Return(nil, io.EOF)

	// Then disconnects from it
	drop := make(chan time.Time)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{}).Return(stream, errors.New("server not available")).WaitUntil(drop).Once()

	// And reconnects
	reconnect := make(chan time.Time)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{}).Return(stream, nil).WaitUntil(reconnect)

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
