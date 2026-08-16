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

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

func baselineConn(host string, bytes uint64) npmodel.NetworkPathConnection {
	return npmodel.NetworkPathConnection{
		Dest:      netip.MustParseAddrPort(host + ":53"),
		Type:      model.ConnectionType_udp,
		Direction: model.ConnectionDirection_outgoing,
		Family:    model.ConnectionFamily_v4,
		Bytes:     bytes,
	}
}

func TestBaselineSelectsCandidatesFromEverySnapshot(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		baselineConn("10.0.0.1", 1),
		baselineConn("10.0.0.2", 3),
		baselineConn("10.0.0.3", 2),
		baselineConn("10.0.0.4", 4),
	}))

	require.Len(t, collector.pathtestInputChan, 3)
	first := <-collector.pathtestInputChan
	assert.Equal(t, payload.DynamicTestProfileBaseline, first.DynamicTestProfile)
	<-collector.pathtestInputChan
	<-collector.pathtestInputChan

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineConn("10.0.1.1", 10)}))
	require.Len(t, collector.pathtestInputChan, 1)
	assert.Equal(t, "10.0.1.1", (<-collector.pathtestInputChan).Hostname)
}

func TestBaselineEmptySnapshotsSelectNothing(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{{
		Dest:      netip.MustParseAddrPort("10.0.0.1:53"),
		Direction: model.ConnectionDirection_incoming,
		Family:    model.ConnectionFamily_v4,
	}}))

	assert.Empty(t, collector.pathtestInputChan)
}

func TestBaselineDisabledCreatesNoCollectorMachinery(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests_enabled": false,
		"network_path.collector.monitor_ip_without_domain":           true,
	}, &teststatsd.Client{}, nil)

	assert.False(t, collector.collectorConfigs.networkPathCollectorEnabled())
	assert.Nil(t, collector.pathtestInputChan)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{baselineConn("10.0.0.1", 1)}))
}

func TestBaselineSelectionConsumesSlotsBeforeChannelAdmission(t *testing.T) {
	stats := &teststatsd.Client{}
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.baseline_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":           true,
		"network_path.collector.input_chan_size":                     1,
	}, stats, nil)

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		baselineConn("10.0.0.1", 3),
		baselineConn("10.0.0.2", 2),
		baselineConn("10.0.0.3", 1),
	}))

	assert.Len(t, collector.pathtestInputChan, 1)
	assert.Equal(t, int64(2), stats.GetCountSummaries()["datadog.network_path.collector.schedule.pathtest_dropped"].Sum)
}
