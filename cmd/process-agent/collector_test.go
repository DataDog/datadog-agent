// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func TestUpdateRTStatus(t *testing.T) {
	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	c, err := NewCollector(cfg, []checks.Check{checks.Process})
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
	assert.True(c.realTimeEnabled.Load())

	// Validate that we stay that way
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.updateRTStatus(statuses)
	assert.True(c.realTimeEnabled.Load())

	// And that it can turn back off
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.updateRTStatus(statuses)
	assert.False(c.realTimeEnabled.Load())
}

func TestUpdateRTInterval(t *testing.T) {
	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	c, err := NewCollector(cfg, []checks.Check{checks.Process})
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
	assert.True(c.realTimeEnabled.Load())
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
			expectedChecks:  []checks.Check{checks.Container},
		},
		{
			name:            "false",
			disableRealtime: false,
			expectedChecks:  []checks.Check{checks.Container, checks.RTContainer},
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.Set("process_config.process_discovery.enabled", false) // Not an RT check so we don't care

			enabledChecks := getChecks(&sysconfig.Config{}, &oconfig.OrchestratorConfig{}, true)
			assert.EqualValues(tc.expectedChecks, enabledChecks)

			c, err := NewCollector(cfg, enabledChecks)
			assert.NoError(err)
			assert.Equal(!tc.disableRealtime, c.runRealTime)
			assert.ElementsMatch(tc.expectedChecks, c.enabledChecks)
		})
	}
}

func TestDisableRealTimeProcessCheck(t *testing.T) {
	tests := []struct {
		name            string
		disableRealtime bool
	}{
		{
			name:            "true",
			disableRealtime: true,
		},
		{
			name:            "false",
			disableRealtime: false,
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig()
	expectedChecks := []checks.Check{checks.Process}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)

			c, err := NewCollector(cfg, expectedChecks)
			assert.NoError(err)
			assert.Equal(!tc.disableRealtime, c.runRealTime)
			assert.EqualValues(expectedChecks, c.enabledChecks)
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
			mockConfig := ddconfig.Mock(t)
			if tc.override {
				mockConfig.Set("process_config.queue_size", tc.queueSize)
			}

			c, err := NewCollector(cfg, []checks.Check{checks.Process, checks.Pod})
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
			mockConfig := ddconfig.Mock(t)
			if tc.override {
				mockConfig.Set("process_config.rt_queue_size", tc.queueSize)
			}

			c, err := NewCollector(cfg, []checks.Check{checks.Process})
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
			mockConfig := ddconfig.Mock(t)
			if tc.override {
				mockConfig.Set("process_config.process_queue_bytes", tc.queueBytes)
			}

			c, err := NewCollector(cfg, []checks.Check{checks.Process})
			assert.NoError(err)
			assert.Equal(int64(tc.expectedQueueSize), c.processResults.MaxWeight())
			assert.Equal(int64(tc.expectedQueueSize), c.rtProcessResults.MaxWeight())
			assert.Equal(tc.expectedQueueSize, c.forwarderRetryQueueMaxBytes)
		})
	}
}

func TestIgnoreResponseBody(t *testing.T) {
	for _, tc := range []struct {
		checkName string
		ignore    bool
	}{
		{checkName: checks.Process.Name(), ignore: false},
		{checkName: checks.Process.RealTimeName(), ignore: false},
		{checkName: checks.ProcessDiscovery.Name(), ignore: false},
		{checkName: checks.Container.Name(), ignore: false},
		{checkName: checks.RTContainer.Name(), ignore: false},
		{checkName: checks.Pod.Name(), ignore: true},
		{checkName: checks.Connections.Name(), ignore: false},
		{checkName: checks.ProcessEvents.Name(), ignore: true},
	} {
		t.Run(tc.checkName, func(t *testing.T) {
			assert.Equal(t, tc.ignore, ignoreResponseBody(tc.checkName))
		})
	}
}
