// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	processMocks "github.com/DataDog/datadog-agent/cmd/process-agent/mocks"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkmocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

func TestUpdateRTStatus(t *testing.T) {
	assert := assert.New(t)
	c, err := NewCollector(nil, &checks.HostInfo{}, []checks.Check{checks.Process})
	assert.NoError(err)
	// XXX: Give the collector a big channel so it never blocks.
	c.rtIntervalCh = make(chan time.Duration, 1000)

	// Validate that we switch to real-time if only one response says so.
	statuses := []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.UpdateRTStatus(statuses)
	assert.True(c.realTimeEnabled.Load())

	// Validate that we stay that way
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.UpdateRTStatus(statuses)
	assert.True(c.realTimeEnabled.Load())

	// And that it can turn back off
	statuses = []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
		{ActiveClients: 0, Interval: 2},
	}
	c.UpdateRTStatus(statuses)
	assert.False(c.realTimeEnabled.Load())
}

func TestUpdateRTInterval(t *testing.T) {
	assert := assert.New(t)
	c, err := NewCollector(nil, &checks.HostInfo{}, []checks.Check{checks.Process})
	assert.NoError(err)
	// XXX: Give the collector a big channel so it never blocks.
	c.rtIntervalCh = make(chan time.Duration, 1000)

	// Validate that we pick the largest interval.
	statuses := []*model.CollectorStatus{
		{ActiveClients: 0, Interval: 3},
		{ActiveClients: 3, Interval: 2},
		{ActiveClients: 0, Interval: 10},
	}
	c.UpdateRTStatus(statuses)
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.Set("process_config.process_discovery.enabled", false) // Not an RT check so we don't care

			enabledChecks := getChecks(&sysconfig.Config{}, true)
			assert.EqualValues(tc.expectedChecks, enabledChecks)

			c, err := NewCollector(nil, &checks.HostInfo{}, enabledChecks)
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
	expectedChecks := []checks.Check{checks.Process}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)

			c, err := NewCollector(nil, &checks.HostInfo{}, expectedChecks)
			assert.NoError(err)
			assert.Equal(!tc.disableRealtime, c.runRealTime)
			assert.EqualValues(expectedChecks, c.enabledChecks)
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
		{checkName: checks.PodCheckManifestName, ignore: true},
		{checkName: checks.Connections.Name(), ignore: false},
		{checkName: checks.ProcessEvents.Name(), ignore: true},
	} {
		t.Run(tc.checkName, func(t *testing.T) {
			assert.Equal(t, tc.ignore, ignoreResponseBody(tc.checkName))
		})
	}
}

func TestCollectorRunCheckWithRealTime(t *testing.T) {
	check := checkmocks.NewCheckWithRealTime(t)

	c, err := NewCollector(nil, &checks.HostInfo{}, []checks.Check{})
	assert.NoError(t, err)
	submitter := processMocks.NewSubmitter(t)
	c.submitter = submitter

	standardOption := checks.RunOptions{
		RunStandard: true,
	}

	result := &checks.RunResult{
		Standard: []model.MessageBody{
			&model.CollectorProc{},
		},
	}

	check.On("RunWithOptions", mock.Anything, standardOption).Once().Return(result, nil)
	check.On("Name").Return("foo")
	check.On("RealTimeName").Return("bar")

	submitStandard := submitter.On("Submit", mock.Anything, check.Name(), mock.Anything).Return(nil)
	submitter.On("Submit", mock.Anything, check.RealTimeName(), mock.Anything).Return(nil).NotBefore(submitStandard)

	c.runCheckWithRealTime(check, standardOption)

	rtResult := &checks.RunResult{
		RealTime: []model.MessageBody{
			&model.CollectorProc{},
		},
	}

	rtOption := checks.RunOptions{
		RunRealTime: true,
	}

	check.On("RunWithOptions", mock.Anything, rtOption).Once().Return(rtResult, nil)

	c.runCheckWithRealTime(check, rtOption)
}

func TestCollectorRunCheck(t *testing.T) {
	check := checkmocks.NewCheck(t)

	hostInfo := &checks.HostInfo{HostName: testHostName}

	c, err := NewCollector(nil, hostInfo, []checks.Check{})
	require.NoError(t, err)
	submitter := processMocks.NewSubmitter(t)
	require.NoError(t, err)
	c.submitter = submitter

	result := []model.MessageBody{
		&model.CollectorProc{},
	}

	check.On("Run", mock.Anything, mock.Anything).Return(result, nil)
	check.On("Name").Return("foo")
	check.On("RealTime").Return(false)
	check.On("ShouldSaveLastRun").Return(true)
	submitter.On("Submit", mock.Anything, check.Name(), mock.Anything).Return(nil)

	c.runCheck(check)
}
