// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"context"
	"io"
	"testing"
	"time"

	payload "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api/mocks"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
)

type eventTestData struct {
	rawEvent     *model.ProcessEvent
	payloadEvent *payload.ProcessEvent
}

func mockedData(t *testing.T) []*eventTestData {
	t.Helper()
	return []*eventTestData{
		{
			rawEvent: &model.ProcessEvent{
				EventType:      model.NewEventType("exec"),
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:10Z"),
				Pid:            42,
				ContainerID:    "0123456789abcdef",
				Ppid:           1,
				UID:            100,
				GID:            100,
				Username:       "user",
				Group:          "mygroup",
				Exe:            "/usr/bin/curl",
				Cmdline: []string{
					"curl",
					"localhost:6062/debug/vars",
				},
				ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:01Z"),
				ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z"),
				ExitTime: time.Time{},
			},
			payloadEvent: &payload.ProcessEvent{
				Type:           payload.ProcEventType_exec,
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:10Z").UnixNano(),
				Pid:            42,
				ContainerId:    "0123456789abcdef",
				Command: &payload.Command{
					Exe:  "/usr/bin/curl",
					Args: []string{"curl", "localhost:6062/debug/vars"},
					Ppid: 1,
				},
				User: &payload.ProcessUser{
					Name: "user",
					Uid:  100,
					Gid:  100,
				},
				TypedEvent: &payload.ProcessEvent_Exec{
					Exec: &payload.ProcessExec{
						ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:01Z").UnixNano(),
						ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z").UnixNano(),
					},
				},
			},
		},
		{
			rawEvent: &model.ProcessEvent{
				EventType:      model.NewEventType("exit"),
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:20Z"),
				Pid:            42,
				ContainerID:    "0123456789abcdef",
				Ppid:           1,
				UID:            100,
				GID:            100,
				Username:       "user",
				Group:          "mygroup",
				Exe:            "/usr/bin/curl",
				Cmdline: []string{
					"curl",
					"localhost:6062/debug/vars",
				},
				ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:01Z"),
				ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z"),
				ExitTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z"),
				ExitCode: 0,
			},
			payloadEvent: &payload.ProcessEvent{
				Type:           payload.ProcEventType_exit,
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:20Z").UnixNano(),
				Pid:            42,
				ContainerId:    "0123456789abcdef",
				Command: &payload.Command{
					Exe:  "/usr/bin/curl",
					Args: []string{"curl", "localhost:6062/debug/vars"},
					Ppid: 1,
				},
				User: &payload.ProcessUser{
					Name: "user",
					Uid:  100,
					Gid:  100,
				},
				TypedEvent: &payload.ProcessEvent_Exit{
					Exit: &payload.ProcessExit{
						ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z").UnixNano(),
						ExitTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z").UnixNano(),
						ExitCode: 0,
					},
				},
			},
		},
		{
			rawEvent: &model.ProcessEvent{
				EventType:      model.NewEventType("exec"),
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:14Z"),
				Pid:            1010,
				ContainerID:    "0123456789abcdef",
				Ppid:           1,
				UID:            100,
				GID:            100,
				Username:       "user",
				Group:          "mygroup",
				Exe:            "/usr/bin/ls",
				Cmdline: []string{
					"ls",
					"invalid-path",
				},
				ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:11Z"),
				ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z"),
				ExitTime: time.Time{},
			},
			payloadEvent: &payload.ProcessEvent{
				Type:           payload.ProcEventType_exec,
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:14Z").UnixNano(),
				Pid:            1010,
				ContainerId:    "0123456789abcdef",
				Command: &payload.Command{
					Exe:  "/usr/bin/ls",
					Args: []string{"ls", "invalid-path"},
					Ppid: 1,
				},
				User: &payload.ProcessUser{
					Name: "user",
					Uid:  100,
					Gid:  100,
				},
				TypedEvent: &payload.ProcessEvent_Exec{
					Exec: &payload.ProcessExec{
						ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:11Z").UnixNano(),
						ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z").UnixNano(),
					},
				},
			},
		},
		{
			rawEvent: &model.ProcessEvent{
				EventType:      model.NewEventType("exit"),
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:14Z"),
				Pid:            1010,
				ContainerID:    "0123456789abcdef",
				Ppid:           1,
				UID:            100,
				GID:            100,
				Username:       "user",
				Group:          "mygroup",
				Exe:            "/usr/bin/ls",
				Cmdline: []string{
					"ls",
					"invalid-path",
				},
				ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:11Z"),
				ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z"),
				ExitTime: parseRFC3339Time(t, "2022-06-12T12:00:13Z"),
				ExitCode: 2,
			},
			payloadEvent: &payload.ProcessEvent{
				Type:           payload.ProcEventType_exit,
				CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:14Z").UnixNano(),
				Pid:            1010,
				ContainerId:    "0123456789abcdef",
				Command: &payload.Command{
					Exe:  "/usr/bin/ls",
					Args: []string{"ls", "invalid-path"},
					Ppid: 1,
				},
				User: &payload.ProcessUser{
					Name: "user",
					Uid:  100,
					Gid:  100,
				},
				TypedEvent: &payload.ProcessEvent_Exit{
					Exit: &payload.ProcessExit{
						ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z").UnixNano(),
						ExitTime: parseRFC3339Time(t, "2022-06-12T12:00:13Z").UnixNano(),
						ExitCode: 2,
					},
				},
			},
		},
	}
}

func TestProcessEventsCheck(t *testing.T) {
	tests := mockedData(t)

	// Initialize check with a mocked gRPC client
	ctx := context.Background()

	client := mocks.NewEventMonitoringModuleClient(t)
	stream := mocks.NewEventMonitoringModule_GetProcessEventsClient(t)
	client.On("GetProcessEvents", ctx, &api.GetProcessEventParams{TimeoutSeconds: 1}).Return(stream, nil)

	for _, test := range tests {
		data, err := test.rawEvent.MarshalMsg(nil)
		require.NoError(t, err)

		stream.On("Recv").Once().Return(&api.ProcessEventMessage{Data: data}, nil)
	}
	stream.On("Recv").Return(nil, io.EOF)

	store, err := events.NewRingStore(ddconfig.Mock(t), &statsd.NoOpClient{})
	require.NoError(t, err)

	listener, err := events.NewSysProbeListener(nil, client, func(e *model.ProcessEvent) {
		_ = store.Push(e, nil)
	})
	require.NoError(t, err)

	check := &ProcessEventsCheck{
		maxBatchSize: 10,
		listener:     listener,
		store:        store,
		hostInfo:     &HostInfo{},
	}
	check.start()

	events := make([]*payload.ProcessEvent, 0)
	assert.Eventually(t, func() bool {
		// Run the process_events check until all expected events are collected
		msgs, err := check.Run(testGroupId(0), nil)
		require.NoError(t, err)

		for _, msg := range msgs.Payloads() {
			collectorProc, ok := msg.(*payload.CollectorProcEvent)
			require.True(t, ok)
			events = append(events, collectorProc.Events...)
		}

		if len(events) == len(tests) {
			for i := range events {
				require.Equal(t, tests[i].payloadEvent, events[i])
			}
			return true
		}

		return false

	}, time.Second, 20*time.Millisecond)

	check.Cleanup()
}

func TestProcessEventsChunking(t *testing.T) {
	for _, tc := range []struct {
		events     int
		chunkSize  int
		chunkCount int
	}{
		{100, 10, 10},
		{50, 30, 2},
		{10, 100, 1},
		{0, 100, 0},
	} {
		events := make([]*payload.ProcessEvent, tc.events)
		chunks := chunkProcessEvents(events, tc.chunkSize)
		assert.Len(t, chunks, tc.chunkCount)
	}
}
