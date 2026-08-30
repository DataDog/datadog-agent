// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"math"
	"net/netip"
	"slices"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func basicConn(host string, bytes uint64) npmodel.NetworkPathConnection {
	return basicConnBytes(host, bytes, 0)
}

func basicConnBytes(host string, sentBytes, recvBytes uint64) npmodel.NetworkPathConnection {
	return npmodel.NetworkPathConnection{
		Dest:      netip.MustParseAddrPort(host + ":53"),
		Type:      model.ConnectionType_udp,
		Direction: model.ConnectionDirection_outgoing,
		Family:    model.ConnectionFamily_v4,
		SentBytes: sentBytes,
		RecvBytes: recvBytes,
	}
}

func basicPath(host string) common.Pathtest {
	return common.Pathtest{Hostname: host, Origin: payload.PathOriginNetworkTraffic}
}

func pathHosts(paths []common.Pathtest) []string {
	hosts := make([]string, len(paths))
	for i, path := range paths {
		hosts[i] = path.Hostname
	}
	return hosts
}

func scheduledBasicHosts(t *testing.T, collector *npCollectorImpl) []string {
	t.Helper()
	hosts := make([]string, 0, len(collector.pathtestInputChan))
	for len(collector.pathtestInputChan) > 0 {
		pathtest := <-collector.pathtestInputChan
		assert.Equal(t, payload.DynamicTestProfileBasic, pathtest.DynamicTestProfile)
		assert.True(t, pathtest.RunOnce)
		hosts = append(hosts, pathtest.Hostname)
	}
	return hosts
}

func TestBasicSelectorAccumulatesTrafficAcrossBootstrapWindow(t *testing.T) {
	now := MockTimeNow()
	selector := newBasicSelector()
	selector.add(basicPath("one"), 40, now)
	selector.add(basicPath("two"), 90, now)
	selector.add(basicPath("one"), 60, now.Add(time.Minute))
	selector.add(basicPath("three"), 80, now.Add(2*time.Minute))
	selector.add(basicPath("four"), 70, now.Add(3*time.Minute))
	selector.add(basicPath("five"), 60, now.Add(4*time.Minute))
	selector.add(basicPath("six"), 50, now.Add(4*time.Minute))

	assert.Nil(t, selector.flush(now.Add(basicBootstrapWindow-time.Second)))
	selected := selector.flush(now.Add(basicBootstrapWindow))

	assert.Equal(t, []string{"one", "two", "three", "four", "five"}, pathHosts(selected))
	for _, path := range selected {
		assert.Equal(t, payload.DynamicTestProfileBasic, path.DynamicTestProfile)
		assert.True(t, path.RunOnce)
	}
}

func TestBasicSelectorUsesHourlyWindowsAfterBootstrap(t *testing.T) {
	now := MockTimeNow()
	selector := newBasicSelector()
	selector.add(basicPath("bootstrap"), 1, now)
	require.Len(t, selector.flush(now.Add(5*time.Minute)), 1)

	selector.add(basicPath("regular"), 1, now.Add(6*time.Minute))
	assert.Nil(t, selector.flush(now.Add(64*time.Minute)))
	selected := selector.flush(now.Add(65 * time.Minute))

	require.Len(t, selected, 1)
	assert.Equal(t, "regular", selected[0].Hostname)
}

func TestBasicSelectorBoundsHeavyHitterCandidates(t *testing.T) {
	now := MockTimeNow()
	selector := newBasicSelector()
	selector.add(basicPath("heavy"), 10_000, now)
	for i := 1; i <= basicCandidateLimit*4; i++ {
		selector.add(basicPath(netip.AddrFrom4([4]byte{10, 0, byte(i / 255), byte(i % 255)}).String()), 1, now)
	}

	assert.Len(t, selector.candidates, basicCandidateLimit)
	selected := selector.flush(now.Add(basicBootstrapWindow))
	require.NotEmpty(t, selected)
	assert.Equal(t, "heavy", selected[0].Hostname)
}

func TestBasicSelectorIgnoresZeroByteObservationsWhenFull(t *testing.T) {
	now := MockTimeNow()
	selector := newBasicSelector()
	originalHashes := make([]uint64, 0, basicCandidateLimit)
	for i := 1; i <= basicCandidateLimit; i++ {
		path := basicPath(netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}).String())
		selector.add(path, 1, now)
		originalHashes = append(originalHashes, path.GetHash())
	}

	for i := 1; i <= basicCandidateLimit*4; i++ {
		path := basicPath(netip.AddrFrom4([4]byte{10, 1, byte(i / 255), byte(i % 255)}).String())
		selector.add(path, 0, now)
	}

	assert.Len(t, selector.candidates, basicCandidateLimit)
	for _, hash := range originalHashes {
		assert.Contains(t, selector.candidates, hash)
	}
}

func TestBasicSelectorBreaksTiesByPathHash(t *testing.T) {
	now := MockTimeNow()
	selector := newBasicSelector()
	paths := []common.Pathtest{basicPath("one"), basicPath("two"), basicPath("three")}
	for _, path := range paths {
		selector.add(path, 1, now)
	}
	slices.SortFunc(paths, func(left, right common.Pathtest) int {
		if left.GetHash() < right.GetHash() {
			return -1
		}
		if left.GetHash() > right.GetHash() {
			return 1
		}
		return 0
	})

	assert.Equal(t, pathHosts(paths), pathHosts(selector.flush(now.Add(basicBootstrapWindow))))
}

func TestSaturatingAdd(t *testing.T) {
	assert.Equal(t, uint64(11), saturatingAdd(5, 6))
	assert.Equal(t, uint64(math.MaxUint64), saturatingAdd(math.MaxUint64-1, 2))
}

func TestBasicCollectorEmitsOnlyAtWindowBoundary(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
	}, &teststatsd.Client{}, nil)
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		basicConn("10.0.0.1", 1),
		basicConn("10.0.0.2", 3),
		basicConn("10.0.0.3", 2),
		basicConn("10.0.0.4", 4),
		basicConn("10.0.0.5", 5),
		basicConn("10.0.0.6", 6),
	}))
	assert.Empty(t, collector.pathtestInputChan)

	now = now.Add(basicBootstrapWindow)
	collector.flushBasicPaths(now)
	assert.Equal(t, []string{"10.0.0.6", "10.0.0.5", "10.0.0.4", "10.0.0.2", "10.0.0.3"}, scheduledBasicHosts(t, collector))
	collector.flushBasicPaths(now.Add(time.Minute))
	assert.Empty(t, collector.pathtestInputChan)
}

func TestBasicCollectorRanksBySentAndReceivedBytes(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
	}, &teststatsd.Client{}, nil)
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		basicConnBytes("10.0.0.1", 100, 0),
		basicConnBytes("10.0.0.2", 0, 60),
	}))
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		basicConnBytes("10.0.0.2", 0, 60),
	}))
	collector.flushBasicPaths(now.Add(basicBootstrapWindow))

	assert.Equal(t, []string{"10.0.0.2", "10.0.0.1"}, scheduledBasicHosts(t, collector))
}

func TestBasicCollectorDoesNotMixBoundarySnapshotIntoPreviousWindow(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
		"network_path.collector.pathtest_interval":                10 * time.Minute,
	}, &teststatsd.Client{}, nil)
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.1", 1)}))
	now = now.Add(basicBootstrapWindow)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.2", 2)}))
	assert.Equal(t, []string{"10.0.0.1"}, scheduledBasicHosts(t, collector))

	now = now.Add(basicSelectionInterval)
	collector.flushBasicPaths(now)
	assert.Equal(t, []string{"10.0.0.2"}, scheduledBasicHosts(t, collector))
}

func TestBasicCollectorPreservesWinningRCProvenance(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
		"network_path.collector.filters": []map[string]any{{
			"type":     "exclude",
			"match_ip": "10.0.0.1",
		}},
	}, &teststatsd.Client{}, nil)
	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"dynamic": {Config: []byte(`{
			"type":"dynamic",
			"test_config_id":"dynamic-a",
			"tags":["team:payments"],
			"config":{"filters":[{"type":"include","match_ip":"10.0.0.1"}]}
		}`)},
	}, func(string, state.ApplyStatus) {})
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.1", 1)}))
	collector.flushBasicPaths(now.Add(basicBootstrapWindow))

	pathtest := <-collector.pathtestInputChan
	assert.Equal(t, "dynamic-a", pathtest.TestConfigID)
	assert.Equal(t, payload.TestConfigSourceRemote, pathtest.TestConfigSource)
	assert.Equal(t, []string{"team:payments"}, pathtest.Tags)
}

func TestBasicIntervalIgnoresPathtestInterval(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.pathtest_interval":                time.Minute,
		"network_path.collector.monitor_ip_without_domain":        true,
	}, &teststatsd.Client{}, nil)
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.1", 1)}))
	collector.flushBasicPaths(now.Add(basicBootstrapWindow))
	require.Len(t, scheduledBasicHosts(t, collector), 1)

	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.2", 1)}))
	collector.flushBasicPaths(now.Add(basicBootstrapWindow + time.Minute))
	assert.Empty(t, collector.pathtestInputChan)
	collector.flushBasicPaths(now.Add(basicBootstrapWindow + basicSelectionInterval))
	assert.Equal(t, []string{"10.0.0.2"}, scheduledBasicHosts(t, collector))
	assert.Equal(t, time.Minute, collector.collectorConfigs.storeConfig.Interval)
}

func TestStandardModeTakesPrecedenceOverBasic(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.enabled":             true,
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
	}, &teststatsd.Client{}, nil)

	assert.Nil(t, collector.basicSelector)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.1", 1)}))
	pathtest := <-collector.pathtestInputChan
	assert.Empty(t, pathtest.DynamicTestProfile)
	assert.False(t, pathtest.RunOnce)
}

func TestBasicEmptyWindowSelectsNothing(t *testing.T) {
	selector := newBasicSelector()
	selector.start(MockTimeNow())
	assert.Empty(t, selector.flush(MockTimeNow().Add(basicBootstrapWindow)))
}

func TestBasicDisabledCreatesNoCollectorMachinery(t *testing.T) {
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": false,
	}, &teststatsd.Client{}, nil)

	assert.False(t, collector.collectorConfigs.networkPathCollectorEnabled())
	assert.Nil(t, collector.basicSelector)
	assert.Nil(t, collector.pathtestInputChan)
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{basicConn("10.0.0.1", 1)}))
}

func TestBasicWindowConsumesSlotsBeforeChannelAdmission(t *testing.T) {
	stats := &teststatsd.Client{}
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.basic_tests_enabled": true,
		"network_path.collector.monitor_ip_without_domain":        true,
		"network_path.collector.input_chan_size":                  1,
	}, stats, nil)
	now := MockTimeNow()
	collector.TimeNowFn = func() time.Time { return now }
	collector.ScheduleNetworkPathTests(slices.Values([]npmodel.NetworkPathConnection{
		basicConn("10.0.0.1", 5),
		basicConn("10.0.0.2", 4),
		basicConn("10.0.0.3", 3),
		basicConn("10.0.0.4", 2),
		basicConn("10.0.0.5", 1),
	}))

	collector.flushBasicPaths(now.Add(basicBootstrapWindow))

	assert.Len(t, collector.pathtestInputChan, 1)
	assert.Equal(t, int64(4), stats.GetCountSummaries()["datadog.network_path.collector.schedule.pathtest_dropped"].Sum)
	collector.flushBasicPaths(now.Add(basicBootstrapWindow + time.Minute))
	assert.Len(t, collector.pathtestInputChan, 1, "dropped winners must not be retried")
}
