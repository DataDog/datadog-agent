// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/process/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkmocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
	processmocks "github.com/DataDog/datadog-agent/pkg/process/runner/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestUpdateRTStatus(t *testing.T) {
	cfg := ddconfig.Mock(t)

	assert := assert.New(t)
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()))
	c, err := NewRunner(cfg, nil, &checks.HostInfo{}, []checks.Check{checks.NewProcessCheck(cfg, cfg, wmeta)}, nil)
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
	cfg := ddconfig.Mock(t)
	assert := assert.New(t)
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()))
	c, err := NewRunner(ddconfig.Mock(t), nil, &checks.HostInfo{}, []checks.Check{checks.NewProcessCheck(cfg, cfg, wmeta)}, nil)
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
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.SetWithoutSource("process_config.disable_realtime_checks", tc.disableRealtime)

			assert := assert.New(t)
			expectedChecks := []checks.Check{checks.NewProcessCheck(mockConfig, mockConfig, wmeta)}

			c, err := NewRunner(mockConfig, nil, &checks.HostInfo{}, expectedChecks, nil)
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
		{checkName: checks.ProcessCheckName, ignore: false},
		{checkName: checks.RTProcessCheckName, ignore: false},
		{checkName: checks.DiscoveryCheckName, ignore: false},
		{checkName: checks.ContainerCheckName, ignore: false},
		{checkName: checks.RTContainerCheckName, ignore: false},
		{checkName: checks.ConnectionsCheckName, ignore: false},
		{checkName: checks.ProcessEventsCheckName, ignore: true},
	} {
		t.Run(tc.checkName, func(t *testing.T) {
			assert.Equal(t, tc.ignore, ignoreResponseBody(tc.checkName))
		})
	}
}

func TestCollectorRunCheckWithRealTime(t *testing.T) {
	check := checkmocks.NewCheck(t)

	c, err := NewRunner(ddconfig.Mock(t), nil, &checks.HostInfo{}, []checks.Check{}, nil)
	assert.NoError(t, err)
	submitter := processmocks.NewSubmitter(t)
	c.Submitter = submitter

	standardOption := &checks.RunOptions{
		RunStandard: true,
	}

	result := checks.StandardRunResult(
		[]model.MessageBody{
			&model.CollectorProc{},
		},
	)

	check.On("Run", mock.Anything, standardOption).Once().Return(result, nil)
	check.On("Name").Return("foo")

	submitStandard := submitter.On("Submit", mock.Anything, check.Name(), mock.Anything).Return(nil)
	submitter.On("Submit", mock.Anything, checks.RTName(check.Name()), mock.Anything).Return(nil).NotBefore(submitStandard)

	c.runCheckWithRealTime(check, standardOption)

	rtResult := checks.CombinedRunResult{
		Realtime: []model.MessageBody{
			&model.CollectorProc{},
		},
	}

	rtOption := &checks.RunOptions{
		RunRealtime: true,
	}

	check.On("Run", mock.Anything, rtOption).Once().Return(rtResult, nil)

	c.runCheckWithRealTime(check, rtOption)
}

func TestCollectorRunCheck(t *testing.T) {
	check := checkmocks.NewCheck(t)

	hostInfo := &checks.HostInfo{HostName: testHostName}

	c, err := NewRunner(ddconfig.Mock(t), nil, hostInfo, []checks.Check{}, nil)
	require.NoError(t, err)
	submitter := processmocks.NewSubmitter(t)
	require.NoError(t, err)
	c.Submitter = submitter

	result := checks.StandardRunResult([]model.MessageBody{
		&model.CollectorProc{},
	})
	check.On("Run", mock.Anything, mock.Anything).Return(result, nil)
	check.On("Name").Return("foo")
	check.On("Realtime").Return(false)
	check.On("ShouldSaveLastRun").Return(true)
	submitter.On("Submit", mock.Anything, check.Name(), mock.Anything).Return(nil)

	c.runCheck(check)
}

// TestSubmitterDoesntBlockOnRTUpdate tests notifyRTStatusChange to ensure that we never block if the channel is filled up
func TestSubmitterDoesntBlockOnRTUpdate(*testing.T) {
	emptyChan := make(chan<- types.RTResponse)
	notifyRTStatusChange(emptyChan, types.RTResponse{})
}
