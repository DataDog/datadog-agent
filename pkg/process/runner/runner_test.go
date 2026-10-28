// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/process/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkmocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
	processmocks "github.com/DataDog/datadog-agent/pkg/process/runner/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestUpdateRTStatus(t *testing.T) {
	cfg := configmock.New(t)

	// Mock IPC component to provide TLS credentials
	ipcMock := ipcmock.New(t)
	taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), hostnameimpl.MockModule(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))

	assert := assert.New(t)
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), hostnameimpl.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	c, err := NewRunner(cfg, nil, &checks.HostInfo{}, []checks.Check{checks.NewProcessCheck(cfg, cfg, wmeta, nil, &statsd.NoOpClient{}, ipcMock.GetTLSServerConfig(), taggerMock)}, nil)
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
	cfg := configmock.New(t)
	assert := assert.New(t)
	// Mock IPC component to provide TLS credentials
	ipcMock := ipcmock.New(t)
	taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), hostnameimpl.MockModule(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), hostnameimpl.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	c, err := NewRunner(configmock.New(t), nil, &checks.HostInfo{}, []checks.Check{checks.NewProcessCheck(cfg, cfg, wmeta, nil, &statsd.NoOpClient{}, ipcMock.GetTLSServerConfig(), taggerMock)}, nil)
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

type statusNamedCheck struct {
	checks.Check
	statusNames []string
}

func (c statusNamedCheck) StatusNames() []string { return c.statusNames }

func TestEnabledCheckNames(t *testing.T) {
	tests := []struct {
		name            string
		checkName       string
		statusNames     []string
		realTimeAllowed bool
		expected        []string
	}{
		{
			name:     "no checks enabled",
			expected: []string{},
		},
		{
			name:            "process collection enabled",
			checkName:       checks.ProcessCheckName,
			statusNames:     []string{checks.ProcessCheckName},
			realTimeAllowed: true,
			expected:        []string{checks.ProcessCheckName, checks.RTProcessCheckName},
		},
		{
			name:            "service discovery enabled",
			checkName:       checks.ProcessCheckName,
			statusNames:     []string{checks.ServiceDiscoveryCheckName},
			realTimeAllowed: true,
			expected:        []string{checks.ServiceDiscoveryCheckName},
		},
		{
			name:            "both features enabled",
			checkName:       checks.ProcessCheckName,
			statusNames:     []string{checks.ProcessCheckName, checks.ServiceDiscoveryCheckName},
			realTimeAllowed: true,
			expected:        []string{checks.ProcessCheckName, checks.RTProcessCheckName, checks.ServiceDiscoveryCheckName},
		},
		{
			name:        "realtime disabled",
			checkName:   checks.ProcessCheckName,
			statusNames: []string{checks.ProcessCheckName},
			expected:    []string{checks.ProcessCheckName},
		},
		{
			name:      "unrelated check remains visible",
			checkName: checks.ConnectionsCheckName,
			expected:  []string{checks.ConnectionsCheckName},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enabledChecks := []checks.Check{}
			if tc.checkName != "" {
				check := checkmocks.NewCheck(t)
				check.On("Name").Return(tc.checkName)
				enabledChecks = append(enabledChecks, check)
				if tc.statusNames != nil {
					enabledChecks[0] = statusNamedCheck{Check: check, statusNames: tc.statusNames}
				}
			}

			actual := enabledCheckNames(enabledChecks, tc.realTimeAllowed)
			assert.Equal(t, tc.expected, actual)
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
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), hostnameimpl.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("process_config.disable_realtime_checks", tc.disableRealtime)

			// Mock IPC component to provide TLS credentials
			ipcMock := ipcmock.New(t)

			assert := assert.New(t)
			taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), hostnameimpl.MockModule(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
			expectedChecks := []checks.Check{checks.NewProcessCheck(mockConfig, mockConfig, wmeta, nil, &statsd.NoOpClient{}, ipcMock.GetTLSServerConfig(), taggerMock)}

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
	} {
		t.Run(tc.checkName, func(t *testing.T) {
			assert.Equal(t, tc.ignore, ignoreResponseBody(tc.checkName))
		})
	}
}

func TestCollectorRunCheckWithRealTime(t *testing.T) {
	check := checkmocks.NewCheck(t)

	c, err := NewRunner(configmock.New(t), nil, &checks.HostInfo{}, []checks.Check{}, nil)
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

	c, err := NewRunner(configmock.New(t), nil, hostInfo, []checks.Check{}, nil)
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

func TestRunnerReadsLatestCoalescedRTStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		c, err := NewRunnerWithChecks(deps.Config, nil, &checks.HostInfo{}, nil, true, s.GetRTNotifier())
		require.NoError(t, err)
		t.Cleanup(c.Stop)

		c.UpdateRTStatus([]*model.CollectorStatus{{ActiveClients: 1, Interval: 5}})
		assert.True(t, c.realTimeEnabled.Load())

		// The runner has not started listening yet. Both publications fit into
		// one wake-up, which must still apply the second (inactive) status.
		s.updateRTStatus("https://example.com", 1, &model.CollectorStatus{ActiveClients: 1, Interval: 5})
		s.updateRTStatus("https://example.com", 2, &model.CollectorStatus{ActiveClients: 0, Interval: 8})
		c.listenForRTUpdates()
		synctest.Wait()
		assert.False(t, c.realTimeEnabled.Load())
		assert.Equal(t, 8*time.Second, c.realTimeInterval)
	})
}

func TestListenerSkipsOlderRTSequence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		updates := make(chan struct{}, 1)
		var mu sync.Mutex
		var latest types.RTResponse
		notifier := &types.RTNotifier{
			Wake: updates,
			Latest: func() types.RTResponse {
				mu.Lock()
				defer mu.Unlock()
				return latest
			},
		}
		publish := func(response types.RTResponse) {
			mu.Lock()
			latest = response
			mu.Unlock()
			updates <- struct{}{}
		}
		c, err := NewRunnerWithChecks(configmock.New(t), nil, &checks.HostInfo{}, nil, true, notifier)
		require.NoError(t, err)
		c.listenForRTUpdates()
		t.Cleanup(c.Stop)

		publish(types.RTResponse{Sequence: 3, Statuses: []*model.CollectorStatus{{ActiveClients: 1, Interval: 5}}})
		synctest.Wait()
		assert.True(t, c.realTimeEnabled.Load())
		assert.Equal(t, 5*time.Second, c.realTimeInterval)

		publish(types.RTResponse{Sequence: 2, Statuses: []*model.CollectorStatus{{ActiveClients: 0, Interval: 2}}})
		synctest.Wait()
		assert.True(t, c.realTimeEnabled.Load(), "an older snapshot must not turn realtime off")
		assert.Equal(t, 5*time.Second, c.realTimeInterval)

		publish(types.RTResponse{Sequence: 4, Statuses: []*model.CollectorStatus{{ActiveClients: 0, Interval: 8}}})
		synctest.Wait()
		assert.False(t, c.realTimeEnabled.Load())
		assert.Equal(t, 8*time.Second, c.realTimeInterval)
	})
}
