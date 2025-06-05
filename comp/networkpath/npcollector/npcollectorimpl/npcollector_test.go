// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/netipx"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func Test_NpCollector_StartAndStop(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	assert.False(t, npCollector.running)

	// TEST START
	app.RequireStart()
	assert.True(t, npCollector.running)

	// TEST START CALLED TWICE
	err = npCollector.start()
	assert.EqualError(t, err, "server already started")

	// TEST STOP
	app.RequireStop()
	assert.False(t, npCollector.running)

	// TEST START/STOP using logs
	l.Close() // We need to first close the logger to avoid a race-cond between seelog and out test when calling w.Flush()
	w.Flush()
	logs := b.String()

	assert.Equal(t, 1, strings.Count(logs, "Start NpCollector"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting listening for pathtests"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting workers"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting worker #0"), logs)

	assert.Equal(t, 1, strings.Count(logs, "Stopped listening for pathtests"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Stopped flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Stop NpCollector"), logs)
}

func Test_NpCollector_runningAndProcessing(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.flush_interval":       "1s",
		"network_devices.namespace":                   "my-ns1",
	}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))
	npCollector.epForwarder = mockEpForwarder

	app.RequireStart()

	assert.True(t, npCollector.running)

	npCollector.runTraceroute = func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error) {
		var p payload.NetworkPath
		if cfg.DestHostname == "10.0.0.2" {
			p = payload.NetworkPath{
				PathtraceID: "pathtrace-id-111",
				Protocol:    payload.ProtocolUDP,
				Source:      payload.NetworkPathSource{Hostname: "abc"},
				Destination: payload.NetworkPathDestination{Hostname: "abc", IPAddress: "10.0.0.2", Port: 80},
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.1"},
					{Hostname: "hop_2", IPAddress: "1.1.1.2"},
				},
			}
		}
		if cfg.DestHostname == "10.0.0.4" {
			p = payload.NetworkPath{
				PathtraceID: "pathtrace-id-222",
				Protocol:    payload.ProtocolUDP,
				Source:      payload.NetworkPathSource{Hostname: "abc"},
				Destination: payload.NetworkPathDestination{Hostname: "abc", IPAddress: "10.0.0.4", Port: 80},
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.3"},
					{Hostname: "hop_2", IPAddress: "1.1.1.4"},
				},
			}
		}
		return p, nil
	}

	// EXPECT
	// language=json
	event1 := []byte(`
{
    "timestamp": 0,
    "agent_version": "",
    "namespace": "my-ns1",
    "pathtrace_id": "pathtrace-id-111",
    "origin":"network_traffic",
    "protocol": "UDP",
    "source": {
        "hostname": "abc",
        "container_id": "testId1"
    },
    "destination": {
        "hostname": "abc",
        "ip_address": "10.0.0.2",
        "port": 80,
		"reverse_dns_hostname": "hostname-10.0.0.2"
    },
    "hops": [
        {
            "ttl": 0,
            "ip_address": "1.1.1.1",
            "hostname": "hop_1",
            "reachable": false
        },
        {
            "ttl": 0,
            "ip_address": "1.1.1.2",
            "hostname": "hop_2",
            "reachable": false
        }
    ]
}
`)
	// language=json
	event2 := []byte(`
{
    "timestamp": 0,
    "agent_version": "",
    "namespace": "my-ns1",
    "pathtrace_id": "pathtrace-id-222",
    "origin":"network_traffic",
    "protocol": "UDP",
    "source": {
        "hostname": "abc",
        "container_id": "testId2"
    },
    "destination": {
        "hostname": "abc",
        "ip_address": "10.0.0.4",
        "port": 80,
		"reverse_dns_hostname": "hostname-10.0.0.4"
    },
    "hops": [
        {
            "ttl": 0,
            "ip_address": "1.1.1.3",
            "hostname": "hop_1",
            "reachable": false
        },
        {
            "ttl": 0,
            "ip_address": "1.1.1.4",
            "hostname": "hop_2",
            "reachable": false
        }
    ]
}
`)
	mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(
		message.NewMessage(compactJSON(event1), nil, "", 0),
		eventplatform.EventTypeNetworkPath,
	).Return(nil).Times(1)

	mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(
		message.NewMessage(compactJSON(event2), nil, "", 0),
		eventplatform.EventTypeNetworkPath,
	).Return(nil).Times(1)

	// WHEN
	conns := []*model.Connection{
		{
			Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000), ContainerId: "testId1"},
			Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
			Type:      model.ConnectionType_tcp,
		},
		{
			Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId2"},
			Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
			Type:      model.ConnectionType_udp,
		},
	}
	npCollector.ScheduleConns(conns, make(map[string]*model.DNSEntry))

	waitForProcessedPathtests(npCollector, 5*time.Second, 2)

	// THEN
	assert.Equal(t, uint64(2), npCollector.processedTracerouteCount.Load())
	assert.Equal(t, uint64(2), npCollector.receivedPathtestCount.Load())

	app.RequireStop()
}

func Test_NpCollector_stopWithoutPanic(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.flush_interval":       "1s",
		"network_path.collector.workers":              100,
		"network_devices.namespace":                   "my-ns1",
	}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	app.RequireStart()

	assert.True(t, npCollector.running)

	npCollector.runTraceroute = func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error) {
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond) // simulate slow processing time, to test for panic
		return payload.NetworkPath{
			PathtraceID: "pathtrace-id-111-" + cfg.DestHostname,
			Protocol:    cfg.Protocol,
			Source:      payload.NetworkPathSource{Hostname: "abc"},
			Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, IPAddress: cfg.DestHostname, Port: cfg.DestPort},
			Hops: []payload.NetworkPathHop{
				{Hostname: "hop_1", IPAddress: "1.1.1.1"},
				{Hostname: "hop_2", IPAddress: "1.1.1.2"},
			},
		}, nil
	}

	// WHEN
	var conns []*model.Connection
	currentIP, _ := netip.ParseAddr("10.0.0.0")
	for i := 0; i < 1000; i++ {
		currentIP = netipx.AddrNext(currentIP)
		conns = append(conns, &model.Connection{
			Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000), ContainerId: "testId1"},
			Raddr:     &model.Addr{Ip: currentIP.String(), Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
			Type:      model.ConnectionType_tcp,
		})
	}
	npCollector.ScheduleConns(conns, make(map[string]*model.DNSEntry))

	waitForProcessedPathtests(npCollector, 5*time.Second, 10)

	// THEN
	assert.GreaterOrEqual(t, int(npCollector.processedTracerouteCount.Load()), 10)

	// test that stop sequence won't trigger panic
	app.RequireStop()
}

func Test_NpCollector_ScheduleConns_ScheduleDurationMetric(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)

	conns := []*model.Connection{
		{
			Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000), ContainerId: "testId1"},
			Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
			Type:      model.ConnectionType_tcp,
		},
		{
			Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId2"},
			Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
			Type:      model.ConnectionType_udp,
		},
	}
	timeNowCounter := 0
	npCollector.TimeNowFn = func() time.Time {
		now := MockTimeNow().Add(time.Duration(timeNowCounter) * time.Minute)
		timeNowCounter++
		return now
	}

	// WHEN
	npCollector.ScheduleConns(conns, make(map[string]*model.DNSEntry))

	// THEN
	calls := stats.GaugeCalls
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.schedule.duration", Value: 60.0, Tags: nil, Rate: 1})
}

func compactJSON(metadataEvent []byte) []byte {
	compactMetadataEvent := new(bytes.Buffer)
	json.Compact(compactMetadataEvent, metadataEvent)
	return compactMetadataEvent.Bytes()
}

func Test_newNpCollectorImpl_defaultConfigs(t *testing.T) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 4, npCollector.workers)
	assert.Equal(t, 1000, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 1000, cap(npCollector.pathtestProcessingChan))
	assert.Equal(t, 5000, npCollector.collectorConfigs.storeConfig.ContextsLimit)
	assert.Equal(t, "default", npCollector.networkDevicesNamespace)
}

func Test_newNpCollectorImpl_overrideConfigs(t *testing.T) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled":    true,
		"network_path.collector.workers":                 2,
		"network_path.collector.input_chan_size":         300,
		"network_path.collector.processing_chan_size":    400,
		"network_path.collector.pathtest_contexts_limit": 500,
		"network_devices.namespace":                      "ns1",
	}

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 2, npCollector.workers)
	assert.Equal(t, 300, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 400, cap(npCollector.pathtestProcessingChan))
	assert.Equal(t, 500, npCollector.collectorConfigs.storeConfig.ContextsLimit)
	assert.Equal(t, "ns1", npCollector.networkDevicesNamespace)
}

func Test_npCollectorImpl_ScheduleConns(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	defaultagentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	tests := []struct {
		name              string
		conns             []*model.Connection
		dns               map[string]*model.DNSEntry
		noInputChan       bool
		agentConfigs      map[string]any
		expectedPathtests []*common.Pathtest
		expectedLogs      []logCount
	}{
		{
			name:              "zero conn",
			agentConfigs:      defaultagentConfigs,
			conns:             []*model.Connection{},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:         "one outgoing TCP conn",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1"},
			},
		},
		{
			name:         "one outgoing UDP conn",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.5", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.6", Port: int32(161)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_udp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.6", Port: uint16(0), Protocol: payload.ProtocolUDP, SourceContainerID: "testId1"},
			},
		},
		{
			name:         "only non-outgoing conns",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
					Type:      model.ConnectionType_tcp,
				},
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId2"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:         "ignore non-outgoing conn",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
					Type:      model.ConnectionType_tcp,
				},
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId2"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId2"},
			},
		},
		{
			name:         "no input chan",
			agentConfigs: defaultagentConfigs,
			noInputChan:  true,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"[ERROR] ScheduleConns: Error scheduling pathtests: no input channel, please check that network path is enabled", 1},
			},
		},
		{
			name: "input chan is full",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled": true,
				"network_path.collector.input_chan_size":      1,
			},
			conns:             createConns(20),
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"collector input channel is full", 10},
			},
		},
		{
			name:         "only ipv4 supported",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "::1", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Family:    model.ConnectionFamily_v6,
					Type:      model.ConnectionType_tcp,
				},
				{
					Laddr:     &model.Addr{Ip: "::1", Port: int32(30000), ContainerId: "testId2"},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Family:    model.ConnectionFamily_v6,
					Type:      model.ConnectionType_tcp,
				},
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId3"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId3"},
			},
			expectedLogs: []logCount{},
		},
		{
			name:         "one outgoing TCP conn with known hostname (DNS)",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "10.0.0.3", Port: int32(30000), ContainerId: "testId1"},
					Raddr:     &model.Addr{Ip: "10.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Type:      model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "known-hostname"}},
			},
			dns: map[string]*model.DNSEntry{
				"10.0.0.4": {Names: []string{"known-hostname"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, tt.agentConfigs, stats)
			if tt.noInputChan {
				npCollector.pathtestInputChan = nil
			}

			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			npCollector.ScheduleConns(tt.conns, tt.dns)

			actualPathtests := []*common.Pathtest{}
			for i := 0; i < len(tt.expectedPathtests); i++ {
				select {
				case pathtest := <-npCollector.pathtestInputChan:
					actualPathtests = append(actualPathtests, pathtest)
				case <-time.After(200 * time.Millisecond):
					assert.Fail(t, fmt.Sprintf("Not enough pathtests: expected=%d but actual=%d", len(tt.expectedPathtests), len(actualPathtests)))
				}
			}

			assert.Equal(t, tt.expectedPathtests, actualPathtests)

			// Flush logs
			w.Flush()
			logs := b.String()

			// Test metrics
			var scheduleDurationMetric teststatsd.MetricsArgs
			calls := stats.GaugeCalls
			for _, call := range calls {
				if call.Name == "datadog.network_path.collector.schedule.duration" {
					scheduleDurationMetric = call
				}
			}
			assert.Less(t, scheduleDurationMetric.Value, float64(5)) // we can't easily assert precise value, hence we are only asserting that it's a low value e.g. 5 seconds
			scheduleDurationMetric.Value = 0                         // We need to reset the metric value to ease testing time duration
			assert.Equal(t, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.schedule.duration", Value: 0, Tags: nil, Rate: 1}, scheduleDurationMetric)

			// Test using logs
			for _, expectedLog := range tt.expectedLogs {
				assert.Equal(t, expectedLog.count, strings.Count(logs, expectedLog.log), logs)
			}
		})
	}
}

func Test_npCollectorImpl_stopWorker(t *testing.T) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{})

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	stopped := make(chan bool, 1)
	go func() {
		npCollector.runWorker(42)
		stopped <- true
	}()
	close(npCollector.stopChan)
	<-stopped

	// Flush logs
	w.Flush()
	logs := b.String()

	assert.Equal(t, 1, strings.Count(logs, "[worker42] Stopped worker"), logs)
}

func Test_npCollectorImpl_flushWrapper(t *testing.T) {
	tests := []struct {
		name               string
		flushStartTime     time.Time
		flushEndTime       time.Time
		lastFlushTime      time.Time
		notExpectedMetrics []string
		expectedMetrics    []teststatsd.MetricsArgs
	}{
		{
			name:           "no last flush time",
			flushStartTime: MockTimeNow(),
			flushEndTime:   MockTimeNow().Add(500 * time.Millisecond),
			notExpectedMetrics: []string{
				"datadog.network_path.collector.flush.interval",
			},
			expectedMetrics: []teststatsd.MetricsArgs{
				{Name: "datadog.network_path.collector.flush.duration", Value: 0.5, Tags: []string{}, Rate: 1},
			},
		},
		{
			name:               "with last flush time",
			flushStartTime:     MockTimeNow(),
			flushEndTime:       MockTimeNow().Add(500 * time.Millisecond),
			lastFlushTime:      MockTimeNow().Add(-2 * time.Minute),
			notExpectedMetrics: []string{},
			expectedMetrics: []teststatsd.MetricsArgs{
				{Name: "datadog.network_path.collector.flush.duration", Value: 0.5, Tags: []string{}, Rate: 1},
				{Name: "datadog.network_path.collector.flush.interval", Value: (2 * time.Minute).Seconds(), Tags: []string{}, Rate: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN
			agentConfigs := map[string]any{
				"network_path.connections_monitoring.enabled": true,
			}
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, agentConfigs, stats)

			npCollector.TimeNowFn = func() time.Time {
				return tt.flushEndTime
			}

			// WHEN
			npCollector.flushWrapper(tt.flushStartTime, tt.lastFlushTime)

			// THEN
			calls := stats.GaugeCalls
			var metricNames []string
			for _, call := range calls {
				metricNames = append(metricNames, call.Name)
			}
			for _, metricName := range tt.notExpectedMetrics {
				assert.NotContains(t, metricNames, metricName)
			}
			for _, metric := range tt.expectedMetrics {
				assert.Contains(t, calls, metric)
			}
		})
	}
}

func Test_npCollectorImpl_flush(t *testing.T) {
	mockNow := time.Now()
	mockTimeNow := func() time.Time {
		return mockNow
	}

	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              6,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)
	npCollector.TimeNowFn = mockTimeNow

	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host1", Port: 53})
	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host2", Port: 53})

	// simulate some time passing so that the PathTestStore rate limit has some budget to work with
	mockNow = mockNow.Add(10 * time.Second)

	// WHEN
	npCollector.flush()

	// THEN
	assert.Contains(t, stats.GaugeCalls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.workers", Value: 6, Tags: []string{}, Rate: 1})
	assert.Contains(t, stats.GaugeCalls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.pathtest_store_size", Value: 2, Tags: []string{}, Rate: 1})
	assert.Contains(t, stats.GaugeCalls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.processing_chan_size", Value: 2, Tags: []string{}, Rate: 1})
	assert.Contains(t, stats.CountCalls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.flush.pathtest_count", Value: 2, Tags: []string{}, Rate: 1})

	assert.Equal(t, 2, len(npCollector.pathtestProcessingChan))
}

func Test_npCollectorImpl_flushLoop(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              6,
		"network_path.collector.flush_interval":       "100ms",
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)
	defer npCollector.stop()

	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host1", Port: 53})
	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host2", Port: 53})

	// WHEN
	go npCollector.flushLoop()

	// THEN
	assert.Eventually(t, func() bool {
		calls := stats.GetGaugeSummaries()["datadog.network_path.collector.flush.interval"]
		if calls == nil {
			return false
		}
		for _, call := range calls.Calls {
			assert.Less(t, call.Value, (200 * time.Millisecond).Seconds())
		}
		return len(calls.Calls) >= 3
	}, 3*time.Second, 10*time.Millisecond)
}

func Benchmark_npCollectorImpl_ScheduleConns(b *testing.B) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              50,
	}

	file, err := os.OpenFile("benchmark.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	assert.Nil(b, err)
	defer file.Close()
	w := bufio.NewWriter(file)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg\n")
	assert.Nil(b, err)
	utillog.SetupLogger(l, "debug")
	defer w.Flush()

	app, npCollector := newTestNpCollector(b, agentConfigs, &teststatsd.Client{})

	// TEST START
	app.RequireStart()
	assert.True(b, npCollector.running)

	// Generate 50 random connections
	connections := createBenchmarkConns(500, 100)

	b.ResetTimer() // Reset timer after setup

	for i := 0; i < b.N; i++ {
		// add line to avoid linter error
		_ = i
		npCollector.ScheduleConns(connections, make(map[string]*model.DNSEntry))

		waitForProcessedPathtests(npCollector, 60*time.Second, 50)
	}

	// TEST STOP
	app.RequireStop()
	assert.False(b, npCollector.running)
}

func Test_npCollectorImpl_enrichPathWithRDNS(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)

	// WHEN
	// Destination, hop 1, hop 3, hop 4 are private IPs, hop 2 is a public IP
	path := payload.NetworkPath{
		Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
		Hops: []payload.NetworkPathHop{
			{IPAddress: "10.0.0.1", Reachable: true, Hostname: "hop1"},
			{IPAddress: "1.1.1.1", Reachable: true, Hostname: "hop2"},
			{IPAddress: "10.0.0.100", Reachable: true, Hostname: "hop3"},
			{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")

	// THEN
	assert.Equal(t, "hostname-10.0.0.41", path.Destination.ReverseDNSHostname) // private IP should be resolved
	assert.Equal(t, "hostname-10.0.0.1", path.Hops[0].Hostname)
	assert.Equal(t, "hop2", path.Hops[1].Hostname) // public IP should fall back to hostname from traceroute
	assert.Equal(t, "hostname-10.0.0.100", path.Hops[2].Hostname)
	assert.Equal(t, "hostname-10.0.0.41", path.Hops[3].Hostname)

	// WHEN
	// hop 3 is a private IP, others are public IPs or unknown hops which should not be resolved
	path = payload.NetworkPath{
		Destination: payload.NetworkPathDestination{IPAddress: "8.8.8.8", Hostname: "google.com"},
		Hops: []payload.NetworkPathHop{
			{IPAddress: "unknown-hop-1", Reachable: false, Hostname: "hop1"},
			{IPAddress: "1.1.1.1", Reachable: true, Hostname: "hop2"},
			{IPAddress: "10.0.0.100", Reachable: true, Hostname: "hop3"},
			{IPAddress: "unknown-hop-4", Reachable: false, Hostname: "hop4"},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")

	// THEN
	assert.Equal(t, "", path.Destination.ReverseDNSHostname)
	assert.Equal(t, "hop1", path.Hops[0].Hostname)
	assert.Equal(t, "hop2", path.Hops[1].Hostname) // public IP should fall back to hostname from traceroute
	assert.Equal(t, "hostname-10.0.0.100", path.Hops[2].Hostname)
	assert.Equal(t, "hop4", path.Hops[3].Hostname)

	// GIVEN - no reverse DNS resolution
	agentConfigs = map[string]any{
		"network_path.connections_monitoring.enabled":           true,
		"network_path.collector.reverse_dns_enrichment.enabled": false,
	}
	_, npCollector = newTestNpCollector(t, agentConfigs, stats)

	// WHEN
	// Destination, hop 1, hop 3, hop 4 are private IPs, hop 2 is a public IP
	path = payload.NetworkPath{
		Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
		Hops: []payload.NetworkPathHop{
			{IPAddress: "10.0.0.1", Reachable: true, Hostname: "hop1"},
			{IPAddress: "1.1.1.1", Reachable: true, Hostname: "hop2"},
			{IPAddress: "10.0.0.100", Reachable: true, Hostname: "hop3"},
			{IPAddress: "10.0.0.41", Reachable: true, Hostname: "dest-hostname"},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")

	// THEN - no resolution should happen
	assert.Equal(t, "", path.Destination.ReverseDNSHostname)
	assert.Equal(t, "hop1", path.Hops[0].Hostname)
	assert.Equal(t, "hop2", path.Hops[1].Hostname)
	assert.Equal(t, "hop3", path.Hops[2].Hostname)
	assert.Equal(t, "dest-hostname", path.Hops[3].Hostname)
}

func Test_npCollectorImpl_enrichPathWithRDNSKnownHostName(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)

	// WHEN
	path := payload.NetworkPath{
		Destination: payload.NetworkPathDestination{IPAddress: "10.0.0.41", Hostname: "dest-hostname"},
		Hops:        nil,
	}

	npCollector.enrichPathWithRDNS(&path, "known-dest-hostname")

	// THEN - destination hostname should resolve to known hostname
	assert.Equal(t, "known-dest-hostname", path.Destination.ReverseDNSHostname)
	assert.Empty(t, path.Hops)
}

func Test_npCollectorImpl_getReverseDNSResult(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats)

	tts := []struct {
		description string
		ipAddr      string
		results     map[string]rdnsquerier.ReverseDNSResult
		expected    string
	}{
		{
			description: "result not in map",
			ipAddr:      "10.0.0.1",
			results:     map[string]rdnsquerier.ReverseDNSResult{},
			expected:    "",
		},
		{
			description: "map is nil",
			ipAddr:      "10.0.0.1",
			results:     nil,
			expected:    "",
		},
		{
			description: "result is an error",
			ipAddr:      "10.0.0.1",
			results: map[string]rdnsquerier.ReverseDNSResult{
				"10.0.0.1": {IP: "10.0.0.1", Hostname: "should-not-be-used", Err: errors.New("error")},
			},
			expected: "",
		},
		{
			description: "result is blank",
			ipAddr:      "10.0.0.1",
			results: map[string]rdnsquerier.ReverseDNSResult{
				"10.0.0.1": {IP: "10.0.0.1", Hostname: ""},
			},
			expected: "",
		},
		{
			description: "result is valid",
			ipAddr:      "10.0.0.1",
			results: map[string]rdnsquerier.ReverseDNSResult{
				"10.0.0.1": {IP: "10.0.0.1", Hostname: "valid-hostname"},
			},
			expected: "valid-hostname",
		},
	}

	for _, tt := range tts {
		t.Run(tt.description, func(t *testing.T) {
			// WHEN
			result := npCollector.getReverseDNSResult(tt.ipAddr, tt.results)

			// THEN
			assert.Equal(t, tt.expected, result)
		})
	}
}

var subnetSkippedStat = teststatsd.MetricsArgs{Name: netpathConnsSkippedMetricName, Value: 1, Tags: []string{"reason:skip_intra_vpc"}, Rate: 1}
var cidrExcludedStat = teststatsd.MetricsArgs{Name: netpathConnsSkippedMetricName, Value: 1, Tags: []string{"reason:skip_cidr_excluded"}, Rate: 1}

func Test_npCollectorImpl_shouldScheduleNetworkPathForConn(t *testing.T) {
	tests := []struct {
		name               string
		conn               *model.Connection
		vpcSubnets         []*net.IPNet
		shouldSchedule     bool
		subnetSkipped      bool
		sourceExcludes     map[string][]string
		destExcludes       map[string][]string
		connectionExcluded bool
	}{
		{
			name: "should schedule",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			shouldSchedule: true,
		},
		{
			name: "should not schedule incoming conn",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_incoming,
				Family:    model.ConnectionFamily_v4,
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule conn with none direction",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_none,
				Family:    model.ConnectionFamily_v4,
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule ipv6",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v6,
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule for loopback",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v4,
				IntraHost: true, // loopback is always IntraHost
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule for intrahost",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v4,
				IntraHost: true,
			},
			shouldSchedule: false,
		},
		// intra-vpc subnet skipping tests
		{
			name: "VPC: random subnet should schedule anyway",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			vpcSubnets:     []*net.IPNet{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "VPC: relevant subnet should skip",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			vpcSubnets:     []*net.IPNet{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		{
			name: "VPC: shouldn't skip local address even if the subnet matches",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			vpcSubnets:     []*net.IPNet{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "VPC: translated clusterIP should get matched",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				IpTranslation: &model.IPTranslation{
					ReplDstPort: int32(80),
					ReplDstIP:   "10.1.2.3",
				},
			},
			vpcSubnets:     []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		{
			name: "VPC: source translation existing shouldn't break subnet check",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				IpTranslation: &model.IPTranslation{
					ReplSrcPort: int32(30000),
					ReplSrcIP:   "192.168.1.2",
					// ReplDstIP is the empty string
				},
			},
			vpcSubnets:     []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		// connection exclusion tests
		{
			name: "exclusion: block dest exactly",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			destExcludes: map[string][]string{
				"10.0.0.2": {"80"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest but different port",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			destExcludes: map[string][]string{
				"10.0.0.2": {"42"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
		{
			name: "exclusion: block source with port range",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			sourceExcludes: map[string][]string{
				"10.0.0.1": {"30000-30005"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest subnet",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			destExcludes: map[string][]string{
				"10.0.0.0/8": {"*"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest subnet, no match",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "192.168.1.1", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			destExcludes: map[string][]string{
				"10.0.0.0/8": {"*"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
		{
			name: "exclusion: only UDP, matching case",
			conn: &model.Connection{
				Type:      model.ConnectionType_udp,
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(123)},
				Direction: model.ConnectionDirection_outgoing,
			},
			sourceExcludes: map[string][]string{
				"10.0.0.0/8": {"udp *"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: only UDP, non-matching case",
			conn: &model.Connection{
				// (tcp is 0 so this doesn't actually do anything)
				Type:      model.ConnectionType_tcp,
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(123)},
				Direction: model.ConnectionDirection_outgoing,
			},
			sourceExcludes: map[string][]string{
				"10.0.0.0/8": {"udp *"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfigs := map[string]any{
				"network_path.connections_monitoring.enabled":         true,
				"network_path.collector.disable_intra_vpc_collection": true,
				"network_path.collector.source_excludes":              tt.sourceExcludes,
				"network_path.collector.dest_excludes":                tt.destExcludes,
			}
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, agentConfigs, stats)

			require.Equal(t, tt.shouldSchedule, npCollector.shouldScheduleNetworkPathForConn(tt.conn, tt.vpcSubnets))

			if tt.subnetSkipped {
				require.Contains(t, stats.CountCalls, subnetSkippedStat)
			} else {
				require.NotContains(t, stats.CountCalls, subnetSkippedStat)
			}
			if tt.connectionExcluded {
				require.Contains(t, stats.CountCalls, cidrExcludedStat)
			} else {
				require.NotContains(t, stats.CountCalls, cidrExcludedStat)
			}
		})
	}
}

func mustParseCIDR(t *testing.T, cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	assert.Nil(t, err)
	return ipNet
}

func Test_npCollectorImpl_shouldScheduleNetworkPathForConn_subnets(t *testing.T) {
	tests := []struct {
		name           string
		conn           *model.Connection
		vpcSubnets     []*net.IPNet
		shouldSchedule bool
		subnetSkipped  bool
	}{
		{
			name: "should schedule",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			vpcSubnets:     nil,
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "should not schedule incoming conn",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "10.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "10.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_incoming,
				Family:    model.ConnectionFamily_v4,
			},
			vpcSubnets:     nil,
			shouldSchedule: false,
			subnetSkipped:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfigs := map[string]any{
				"network_path.connections_monitoring.enabled":         true,
				"network_path.collector.disable_intra_vpc_collection": true,
			}
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, agentConfigs, stats)

			assert.Equal(t, tt.shouldSchedule, npCollector.shouldScheduleNetworkPathForConn(tt.conn, nil))

			if tt.subnetSkipped {
				require.Contains(t, stats.CountCalls, subnetSkippedStat)
			} else {
				require.NotContains(t, stats.CountCalls, subnetSkippedStat)
			}
		})
	}

}
