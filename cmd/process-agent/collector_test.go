// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"sync/atomic"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
)

func TestUpdateRTStatus(t *testing.T) {
	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	c, err := NewCollector(cfg)
	assert.NoError(err)
	// XXX: Give the collector a big channel so it never blocks.
	c.rtIntervalCh = make(chan time.Duration, 1000)

	// Validate that we switch to real-time if only one response says so.
	statuses := []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.updateRTStatus(statuses)
	assert.Equal(int32(1), atomic.LoadInt32(&c.realTimeEnabled))

	// Validate that we stay that way
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.updateRTStatus(statuses)
	assert.Equal(int32(1), atomic.LoadInt32(&c.realTimeEnabled))

	// And that it can turn back off
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.updateRTStatus(statuses)
	assert.Equal(int32(0), atomic.LoadInt32(&c.realTimeEnabled))
}

func TestUpdateRTInterval(t *testing.T) {
	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	c, err := NewCollector(cfg)
	assert.NoError(err)
	// XXX: Give the collector a big channel so it never blocks.
	c.rtIntervalCh = make(chan time.Duration, 1000)

	// Validate that we pick the largest interval.
	statuses := []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 3},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 10},
	}
	c.updateRTStatus(statuses)
	assert.Equal(int32(1), atomic.LoadInt32(&c.realTimeEnabled))
	assert.Equal(10*time.Second, c.realTimeInterval)
}

func TestHasContainers(t *testing.T) {
	assert := assert.New(t)

	collectorProc := model.CollectorProc{}
	collectorContainer := model.CollectorContainer{}
	collectorRealTime := model.CollectorRealTime{}
	collectorContainerRealTime := model.CollectorContainerRealTime{}
	collectorConnections := model.CollectorConnections{}

	assert.Equal(0, getContainerCount(&collectorProc))
	assert.Equal(0, getContainerCount(&collectorContainer))
	assert.Equal(0, getContainerCount(&collectorRealTime))
	assert.Equal(0, getContainerCount(&collectorContainerRealTime))
	assert.Equal(0, getContainerCount(&collectorConnections))

	c := &model.Container{Type: "Docker"}
	cs, cs2 := &model.ContainerStat{Id: "1234"}, &model.ContainerStat{Id: "5678"}

	collectorProc.Containers = append(collectorProc.Containers, c)
	collectorContainer.Containers = append(collectorContainer.Containers, c)
	collectorRealTime.ContainerStats = append(collectorRealTime.ContainerStats, cs, cs2)
	collectorContainerRealTime.Stats = append(collectorContainerRealTime.Stats, cs)

	assert.Equal(1, getContainerCount(&collectorProc))
	assert.Equal(1, getContainerCount(&collectorContainer))
	assert.Equal(2, getContainerCount(&collectorRealTime))
	assert.Equal(1, getContainerCount(&collectorContainerRealTime))
}

func TestDisableRealTime(t *testing.T) {
	tests := []struct {
		name            string
		disableRealtime bool
		expectedChecks  []checks.Check
	}{
		{
			name:            "true",
			disableRealtime: true,
			expectedChecks:  []checks.Check{checks.Process, checks.Container},
		},
		{
			name:            "false",
			disableRealtime: false,
			expectedChecks:  []checks.Check{checks.Process, checks.Container, checks.RTContainer},
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	cfg.EnabledChecks = []string{
		config.ProcessCheckName,
		config.RTProcessCheckName,
		config.ContainerCheckName,
		config.RTContainerCheckName,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)

			c, err := NewCollector(cfg)
			assert.NoError(err)
			assert.ElementsMatch(tc.expectedChecks, c.enabledChecks)
			assert.Equal(!tc.disableRealtime, c.runRealTime)
		})
	}
}

func TestNewCollectorQueueSize(t *testing.T) {
	tests := []struct {
		name              string
		override          bool
		queueSize         int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			override:          false,
			queueSize:         42,
			expectedQueueSize: ddconfig.DefaultProcessQueueSize,
		},
		{
			name:              "valid queue size override",
			override:          true,
			queueSize:         42,
			expectedQueueSize: 42,
		},
		{
			name:              "invalid negative queue size override",
			override:          true,
			queueSize:         -10,
			expectedQueueSize: ddconfig.DefaultProcessQueueSize,
		},
		{
			name:              "invalid 0 queue size override",
			override:          true,
			queueSize:         0,
			expectedQueueSize: ddconfig.DefaultProcessQueueSize,
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.queue_size", tc.queueSize)
			}

			c, err := NewCollector(cfg)
			assert.NoError(err)
			assert.Equal(tc.expectedQueueSize, c.processResults.MaxSize())
			assert.Equal(tc.expectedQueueSize, c.podResults.MaxSize())
		})
	}
}

func TestNewCollectorRTQueueSize(t *testing.T) {
	tests := []struct {
		name              string
		override          bool
		queueSize         int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			override:          false,
			queueSize:         2,
			expectedQueueSize: ddconfig.DefaultProcessRTQueueSize,
		},
		{
			name:              "valid queue size override",
			override:          true,
			queueSize:         2,
			expectedQueueSize: 2,
		},
		{
			name:              "invalid negative size override",
			override:          true,
			queueSize:         -2,
			expectedQueueSize: ddconfig.DefaultProcessRTQueueSize,
		},
		{
			name:              "invalid 0 queue size override",
			override:          true,
			queueSize:         0,
			expectedQueueSize: ddconfig.DefaultProcessRTQueueSize,
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.rt_queue_size", tc.queueSize)
			}

			c, err := NewCollector(cfg)
			assert.NoError(err)
			assert.Equal(tc.expectedQueueSize, c.rtProcessResults.MaxSize())
		})
	}
}

func TestNewCollectorProcessQueueBytes(t *testing.T) {
	tests := []struct {
		name              string
		override          bool
		queueBytes        int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			override:          false,
			queueBytes:        42000,
			expectedQueueSize: ddconfig.DefaultProcessQueueBytes,
		},
		{
			name:              "valid queue size override",
			override:          true,
			queueBytes:        42000,
			expectedQueueSize: 42000,
		},
		{
			name:              "invalid negative queue size override",
			override:          true,
			queueBytes:        -2,
			expectedQueueSize: ddconfig.DefaultProcessQueueBytes,
		},
		{
			name:              "invalid 0 queue size override",
			override:          true,
			queueBytes:        0,
			expectedQueueSize: ddconfig.DefaultProcessQueueBytes,
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.process_queue_bytes", tc.queueBytes)
			}

			c, err := NewCollector(cfg)
			assert.NoError(err)
			assert.Equal(int64(tc.expectedQueueSize), c.processResults.MaxWeight())
			assert.Equal(int64(tc.expectedQueueSize), c.rtProcessResults.MaxWeight())
			assert.Equal(tc.expectedQueueSize, c.forwarderRetryQueueMaxBytes)
		})
	}
}
