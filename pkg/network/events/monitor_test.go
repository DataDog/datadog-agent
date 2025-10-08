// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package events handles process events
package events

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestEventConsumerWrapperCopy(t *testing.T) {
	Init()

	t.Run("test exec process attributes", func(t *testing.T) {
		now := time.Now()
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.ExecEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 2233,
						},
						ExecTime: now,
						Envp: []string{
							"DD_ENV=env",
							"DD_SERVICE=service",
							"DD_VERSION=version",
						},
					},
				},
				ContainerContext: &model.ContainerContext{
					ContainerID: "cid_exec",
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			CGroupContext: &model.CGroupContext{
				CGroupID: "cid_exec",
			},
		}
		evHandler := &eventConsumerWrapper{}
		_p := evHandler.Copy(ev)
		require.IsType(t, &Process{}, _p, "Copy should return a *events.Process")
		p := _p.(*Process)
		assert.Equal(t, uint32(2233), p.Pid)
		assert.Equal(t, now.UnixNano(), p.StartTime)
		assert.EqualValues(t, []*intern.Value{
			intern.GetByString("env:env"),
			intern.GetByString("service:service"),
			intern.GetByString("version:version"),
		}, p.Tags)
		assert.NotNil(t, p.ContainerID, "container ID should not be nil")
		assert.Equal(t, "cid_exec", p.ContainerID.Get().(string), "container id mismatch")
	})

	t.Run("test fork process attributes", func(t *testing.T) {
		now := time.Now()
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.ForkEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 2244,
						},
						ForkTime: now,
						Envp: []string{
							"DD_ENV=env",
							"DD_SERVICE=service",
							"DD_VERSION=version",
						},
					},
				},
				ContainerContext: &model.ContainerContext{
					ContainerID: "cid_fork",
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			CGroupContext: &model.CGroupContext{
				CGroupID: "cid_fork",
			},
		}
		evHandler := &eventConsumerWrapper{}
		_p := evHandler.Copy(ev)
		require.IsType(t, &Process{}, _p, "Copy should return a *events.Process")
		p := _p.(*Process)
		assert.Equal(t, uint32(2244), p.Pid)
		assert.Equal(t, now.UnixNano(), p.StartTime)
		assert.EqualValues(t, []*intern.Value{
			intern.GetByString("env:env"),
			intern.GetByString("service:service"),
			intern.GetByString("version:version"),
		}, p.Tags)
		assert.NotNil(t, p.ContainerID, "container ID should not be nil")
		assert.Equal(t, "cid_fork", p.ContainerID.Get().(string), "container id mismatch")
	})

	t.Run("no container context", func(t *testing.T) {
		ev := &model.Event{BaseEvent: model.BaseEvent{}}
		evHandler := &eventConsumerWrapper{}
		p := evHandler.Copy(ev)
		require.IsType(t, &Process{}, p, "Copy should return a *events.Process")
		assert.Nil(t, p.(*Process).ContainerID, "container ID should be nil")
	})

	t.Run("test tracer_memfd_sealed event attributes", func(t *testing.T) {
		now := time.Now()
		forkTime := now.Add(-5 * time.Second)
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.TracerMemfdSealEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 1234,
						},
						ExecTime: now,
						ForkTime: forkTime,
						Envp: []string{
							"DD_ENV=env",
							"DD_SERVICE=service",
							"DD_VERSION=version",
						},
					},
				},
				ContainerContext: &model.ContainerContext{
					ContainerID: "cid_tracer_memfd",
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			CGroupContext: &model.CGroupContext{
				CGroupID: "cid_tracer_memfd",
			},
			TracerMemfdSeal: model.TracerMemfdSealEvent{
				Fd: 5,
			},
		}
		evHandler := &eventConsumerWrapper{}
		_m := evHandler.Copy(ev)
		require.IsType(t, &TracerMemfdSeal{}, _m, "Copy should return a *events.TracerMemfdSeal")
		m := _m.(*TracerMemfdSeal)
		assert.Equal(t, uint32(5), m.Fd)
		require.NotNil(t, m.Process, "Process should not be nil")
		assert.Equal(t, uint32(1234), m.Process.Pid)
		assert.Equal(t, now.UnixNano(), m.Process.StartTime)
		assert.EqualValues(t, []*intern.Value{
			intern.GetByString("env:env"),
			intern.GetByString("service:service"),
			intern.GetByString("version:version"),
		}, m.Process.Tags)
		assert.NotNil(t, m.Process.ContainerID, "container ID should not be nil")
		assert.Equal(t, "cid_tracer_memfd", m.Process.ContainerID.Get().(string), "container id mismatch")
	})

	t.Run("test tracer_memfd_sealed uses ExecTime when newer", func(t *testing.T) {
		now := time.Now()
		execTime := now.Add(5 * time.Second)
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.TracerMemfdSealEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 1234,
						},
						ForkTime: now,
						ExecTime: execTime,
					},
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			TracerMemfdSeal: model.TracerMemfdSealEvent{
				Fd: 5,
			},
		}
		evHandler := &eventConsumerWrapper{}
		_m := evHandler.Copy(ev)
		require.IsType(t, &TracerMemfdSeal{}, _m, "Copy should return a *events.TracerMemfdSeal")
		m := _m.(*TracerMemfdSeal)
		assert.Equal(t, execTime.UnixNano(), m.Process.StartTime, "StartTime should be ExecTime when ExecTime is after ForkTime")
	})

	t.Run("test tracer_memfd_sealed uses ForkTime when newer", func(t *testing.T) {
		now := time.Now()
		forkTime := now.Add(5 * time.Second)
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.TracerMemfdSealEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 1234,
						},
						ExecTime: now,
						ForkTime: forkTime,
					},
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			TracerMemfdSeal: model.TracerMemfdSealEvent{
				Fd: 5,
			},
		}
		evHandler := &eventConsumerWrapper{}
		_m := evHandler.Copy(ev)
		require.IsType(t, &TracerMemfdSeal{}, _m, "Copy should return a *events.TracerMemfdSeal")
		m := _m.(*TracerMemfdSeal)
		assert.Equal(t, forkTime.UnixNano(), m.Process.StartTime, "StartTime should be ForkTime when ForkTime is after ExecTime")
	})

}

// mockProcessEventHandler is a test handler that records Process events
type mockProcessEventHandler struct {
	events []*Process
}

func (m *mockProcessEventHandler) HandleProcessEvent(p *Process) {
	m.events = append(m.events, p)
}

func TestEventHandleTracerMemfdSeal(t *testing.T) {
	require.NoError(t, Init())

	handler := &mockProcessEventHandler{}
	RegisterHandler(handler)
	defer UnregisterHandler(handler)

	evHandler := &eventConsumerWrapper{}
	pid := uint32(os.Getpid())

	t.Run("tracer memfd with valid metadata creates process event", func(t *testing.T) {
		handler.events = nil // reset

		metadata := &tracermetadata.TracerMetadata{
			ServiceName:    "my-service",
			ServiceEnv:     "my-env",
			ServiceVersion: "my-version",
			ProcessTags:    "entrypoint.name:my-entrypoint",
		}
		data, err := metadata.MarshalMsg(nil)
		require.NoError(t, err)
		fd := createTestMemfd(t, data)

		memfdEvent := &TracerMemfdSeal{
			Fd: uint32(fd),
			Process: &Process{
				Pid: pid,
				Tags: []*intern.Value{
					intern.GetByString("service:from-ust"),
				},
			},
		}

		evHandler.HandleEvent(memfdEvent)

		require.Len(t, handler.events, 1, "should have received 1 process event")
		p := handler.events[0]
		assert.Equal(t, pid, p.Pid)
		assert.Contains(t, p.Tags, intern.GetByString("service:from-ust"))
		assert.Contains(t, p.Tags, intern.GetByString("service:my-service"))
		assert.Contains(t, p.Tags, intern.GetByString("env:my-env"))
		assert.Contains(t, p.Tags, intern.GetByString("version:my-version"))
		assert.Contains(t, p.Tags, intern.GetByString("entrypoint.name:my-entrypoint"))
	})

	t.Run("tracer memfd with invalid fd does not create process event", func(t *testing.T) {
		handler.events = nil // reset

		memfdEvent := &TracerMemfdSeal{
			Fd: 9999, // invalid fd
			Process: &Process{
				Pid: pid,
			},
		}

		evHandler.HandleEvent(memfdEvent)

		assert.Empty(t, handler.events, "should not have received any process events with invalid fd")
	})
}

func createTestMemfd(t *testing.T, data []byte) int {
	t.Helper()
	fd, err := unix.MemfdCreate("datadog-tracer-info-01234567", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(fd) })

	if len(data) > 0 {
		_, err = unix.Write(fd, data)
		require.NoError(t, err)
		// Seal the memfd
		_, err = unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, unix.F_SEAL_WRITE|unix.F_SEAL_SHRINK|unix.F_SEAL_GROW)
		require.NoError(t, err)
	}

	return fd
}
