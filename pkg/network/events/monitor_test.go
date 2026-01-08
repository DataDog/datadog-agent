// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package events handles process events
package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

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
						ContainerContext: model.ContainerContext{
							ContainerID: "cid_exec",
						},
						CGroup: model.CGroupContext{
							CGroupID: "cid_exec",
						},
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
				FieldHandlers: &model.FakeFieldHandlers{},
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
						ContainerContext: model.ContainerContext{
							ContainerID: "cid_fork",
						},
						CGroup: model.CGroupContext{
							CGroupID: "cid_fork",
						},
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
				FieldHandlers: &model.FakeFieldHandlers{},
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
						ContainerContext: model.ContainerContext{
							ContainerID: "cid_tracer_memfd",
						},
						CGroup: model.CGroupContext{
							CGroupID: "cid_tracer_memfd",
						},
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
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			TracerMemfdSeal: model.TracerMemfdSealEvent{
				Fd: 5,
			},
		}
		evHandler := &eventConsumerWrapper{}
		_m := evHandler.Copy(ev)
		require.IsType(t, &Process{}, _m, "Copy should return a *events.Process")
		m := _m.(*Process)
		require.NotNil(t, m, "Process should not be nil")
		assert.Equal(t, uint32(1234), m.Pid)
		assert.Equal(t, now.UnixNano(), m.StartTime)
		assert.EqualValues(t, []*intern.Value{
			intern.GetByString("env:env"),
			intern.GetByString("service:service"),
			intern.GetByString("version:version"),
		}, m.Tags)
		assert.NotNil(t, m.ContainerID, "container ID should not be nil")
		assert.Equal(t, "cid_tracer_memfd", m.ContainerID.Get().(string), "container id mismatch")
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
		require.IsType(t, &Process{}, _m, "Copy should return a *events.Process")
		m := _m.(*Process)
		assert.Equal(t, execTime.UnixNano(), m.StartTime, "StartTime should be ExecTime when ExecTime is after ForkTime")
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
		require.IsType(t, &Process{}, _m, "Copy should return a *events.Process")
		m := _m.(*Process)
		assert.Equal(t, forkTime.UnixNano(), m.StartTime, "StartTime should be ForkTime when ForkTime is after ExecTime")
	})

}

// mockProcessEventHandler is a test handler that records Process events
type mockProcessEventHandler struct {
	events []*Process
}

func (m *mockProcessEventHandler) HandleProcessEvent(p *Process) {
	m.events = append(m.events, p)
}

func TestEventHandleTracerTags(t *testing.T) {
	require.NoError(t, Init())

	handler := &mockProcessEventHandler{}
	RegisterHandler(handler)
	defer UnregisterHandler(handler)

	evHandler := &eventConsumerWrapper{}

	t.Run("process event with tracer tags", func(t *testing.T) {
		handler.events = nil // reset

		now := time.Now()
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.ExecEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						ContainerContext: model.ContainerContext{
							ContainerID: "test-container",
						},
						PIDContext: model.PIDContext{
							Pid: 1234,
						},
						ExecTime: now,
						Envp: []string{
							"DD_SERVICE=service-from-envp",
							"DD_ENV=env-from-envp",
							"DD_VERSION=version-from-envp",
						},
						TracerTags: []string{
							"tracer_service_name:my-service",
							"tracer_service_env:my-env",
							"tracer_service_version:my-version",
							"entrypoint.name:my-entrypoint",
							// Should be skipped because it matches the UST tags
							"tracer_service_name:service-from-envp",
							"tracer_service_env:env-from-envp",
							"tracer_service_version:version-from-envp",
						},
					},
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			},
		}

		p := evHandler.Copy(ev).(*Process)
		evHandler.HandleEvent(p)

		require.Len(t, handler.events, 1, "should have received 1 process event")
		receivedProc := handler.events[0]
		assert.Equal(t, uint32(1234), receivedProc.Pid)
		assert.Contains(t, receivedProc.Tags, intern.GetByString("service:service-from-envp"))
		assert.Contains(t, receivedProc.Tags, intern.GetByString("env:env-from-envp"))
		assert.Contains(t, receivedProc.Tags, intern.GetByString("tracer_service_name:my-service"))
		assert.Contains(t, receivedProc.Tags, intern.GetByString("tracer_service_env:my-env"))
		assert.Contains(t, receivedProc.Tags, intern.GetByString("tracer_service_version:my-version"))
		assert.NotContains(t, receivedProc.Tags, intern.GetByString("tracer_service_name:service-from-envp"))
		assert.NotContains(t, receivedProc.Tags, intern.GetByString("tracer_service_env:env-from-envp"))
		assert.NotContains(t, receivedProc.Tags, intern.GetByString("tracer_service_version:version-from-envp"))
		assert.Contains(t, receivedProc.Tags, intern.GetByString("entrypoint.name:my-entrypoint"))
	})

	t.Run("process event without tracer tags", func(t *testing.T) {
		handler.events = nil // reset

		now := time.Now()
		ev := &model.Event{
			BaseEvent: model.BaseEvent{
				Type: uint32(model.ExecEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{
							Pid: 5678,
						},
						ExecTime: now,
					},
				},
				ProcessCacheEntry: &model.ProcessCacheEntry{},
				FieldHandlers:     &model.FakeFieldHandlers{},
			},
		}

		p := evHandler.Copy(ev).(*Process)
		evHandler.HandleEvent(p)

		require.Len(t, handler.events, 1, "should have received 1 process event")
		receivedProc := handler.events[0]
		assert.Equal(t, uint32(5678), receivedProc.Pid)
		assert.Empty(t, receivedProc.Tags, "should have no tags")
	})
}
