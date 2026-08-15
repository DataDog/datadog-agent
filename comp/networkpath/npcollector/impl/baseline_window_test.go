// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/connfilter"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

func baselineWindowConn(host string, bytes uint64) npmodel.NetworkPathConnection {
	return npmodel.NetworkPathConnection{
		Dest:      netip.MustParseAddrPort(host + ":53"),
		Type:      model.ConnectionType_udp,
		Direction: model.ConnectionDirection_outgoing,
		Family:    model.ConnectionFamily_v4,
		Bytes:     bytes,
	}
}

func TestBaselineFirstSnapshotAndSteadyWindow(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests.enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
		"network_path.collector.pathtest_interval":                   time.Second,
	}, &teststatsd.Client{}, nil)
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	collector.TimeNowFn = func() time.Time { return now }

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		baselineWindowConn("10.0.0.1", 1),
		baselineWindowConn("10.0.0.2", 3),
		baselineWindowConn("10.0.0.3", 2),
		baselineWindowConn("10.0.0.4", 4),
	}))

	require.Len(t, collector.pathtestInputChan, 3, "the virtual first window must select immediately")
	first := <-collector.pathtestInputChan
	assert.Equal(t, payload.DynamicTestProfileBaseline, first.DynamicTestProfile)
	assert.True(t, first.OneShot)
	assert.Equal(t, now.Add(30*time.Minute), first.ExecutionDeadline)
	assert.Equal(t, now.Add(30*time.Minute), collector.baselineDeadline, "baseline must not inherit pathtest_interval")
	<-collector.pathtestInputChan
	<-collector.pathtestInputChan

	now = now.Add(time.Minute)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineWindowConn("10.0.1.1", 10)}))
	assert.Empty(t, collector.pathtestInputChan)

	now = now.Add(29 * time.Minute)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineWindowConn("10.0.2.1", 1)}))
	require.Len(t, collector.pathtestInputChan, 1, "the completed steady-state window must close once")
	steady := <-collector.pathtestInputChan
	assert.Equal(t, "10.0.1.1", steady.Hostname)
	assert.Equal(t, now.Add(30*time.Minute), steady.ExecutionDeadline)
}

func TestBaselineEmptySnapshotsDoNotStartVirtualWindow(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests.enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{{
		Dest:      netip.MustParseAddrPort("10.0.0.1:53"),
		Direction: model.ConnectionDirection_incoming,
		Family:    model.ConnectionFamily_v4,
	}}))

	assert.False(t, collector.baselineStarted)
	assert.Empty(t, collector.pathtestInputChan)
}

func TestBaselineDisabledCreatesNoCollectorMachinery(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests.enabled": false,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)

	assert.False(t, collector.collectorConfigs.networkPathCollectorEnabled())
	assert.Nil(t, collector.pathtestInputChan)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineWindowConn("10.0.0.1", 1)}))
	assert.False(t, collector.baselineStarted)
}

func TestBaselineSelectionConsumesSlotsBeforeChannelAdmission(t *testing.T) {
	stats := &teststatsd.Client{}
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests.enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
		"network_path.collector.input_chan_size":                     1,
	}, stats, nil)

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		baselineWindowConn("10.0.0.1", 3),
		baselineWindowConn("10.0.0.2", 2),
		baselineWindowConn("10.0.0.3", 1),
	}))

	assert.Len(t, collector.pathtestInputChan, 1)
	assert.Equal(t, int64(3), stats.GetCountSummaries()["datadog.network_path.collector.baseline.selections"].Sum,
		"all selected slots are consumed even when downstream admission drops tests")
	assert.Equal(t, int64(2), stats.GetCountSummaries()["datadog.network_path.collector.schedule.pathtest_dropped"].Sum)
}

func TestBaselineBypassesRemoteFiltersButStandardDoesNot(t *testing.T) {
	remoteExclude := []connfilter.Config{{Type: connfilter.FilterTypeExclude, MatchIP: "10.0.0.1"}}

	_, baseline := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests.enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)
	baseline.replaceRemoteFilters(remoteExclude)
	baseline.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineWindowConn("10.0.0.1", 1)}))
	require.Len(t, baseline.pathtestInputChan, 1, "mutable remote filters must not change baseline eligibility")

	_, standard := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.enabled":      true,
		"network_path.collector.monitor_ip_without_domain": true,
	}, &teststatsd.Client{}, nil)
	standard.replaceRemoteFilters(remoteExclude)
	standard.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineWindowConn("10.0.0.1", 1)}))
	assert.Empty(t, standard.pathtestInputChan, "standard tests must continue to honor remote filters")
}
