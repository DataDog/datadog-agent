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
				CGroupContext: model.CGroupContext{
					CGroupID: "cid_exec",
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			}}
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
				CGroupContext: model.CGroupContext{
					CGroupID: "cid_fork",
				},
				FieldHandlers: &model.FakeFieldHandlers{},
			}}
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

}
