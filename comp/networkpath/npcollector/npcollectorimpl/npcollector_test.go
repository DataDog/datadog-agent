// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func Test_NpCollector_StartAndStop(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	app, npCollector := newTestNpCollector(t, agentConfigs)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
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
	}
	app, npCollector := newTestNpCollector(t, agentConfigs)

	stats := &teststatsd.Client{}

	mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))
	npCollector.epForwarder = mockEpForwarder

	app.RequireStart()

	npCollector.statsdClient = stats
	npCollector.metricSender = metricsender.NewMetricSenderStatsd(stats)

	assert.True(t, npCollector.running)

	npCollector.runTraceroute = func(cfg traceroute.Config) (payload.NetworkPath, error) {
		var p payload.NetworkPath
		if cfg.DestHostname == "127.0.0.2" {
			p = payload.NetworkPath{
				Protocol:    payload.ProtocolUDP,
				Source:      payload.NetworkPathSource{Hostname: "abc"},
				Destination: payload.NetworkPathDestination{Hostname: "abc", IPAddress: "127.0.0.2", Port: 80},
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.1"},
					{Hostname: "hop_2", IPAddress: "1.1.1.2"},
				},
			}
		}
		if cfg.DestHostname == "127.0.0.4" {
			p = payload.NetworkPath{
				Protocol:    payload.ProtocolUDP,
				Source:      payload.NetworkPathSource{Hostname: "abc"},
				Destination: payload.NetworkPathDestination{Hostname: "abc", IPAddress: "127.0.0.4", Port: 80},
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
    "namespace": "",
    "path_id": "",
    "protocol": "UDP",
    "source": {
        "hostname": "abc",
        "via": null,
        "network_id": ""
    },
    "destination": {
        "hostname": "abc",
        "ip_address": "127.0.0.2",
        "port": 80
    },
    "hops": [
        {
            "ttl": 0,
            "ip_address": "1.1.1.1",
            "hostname": "hop_1",
            "rtt": 0,
            "success": false
        },
        {
            "ttl": 0,
            "ip_address": "1.1.1.2",
            "hostname": "hop_2",
            "rtt": 0,
            "success": false
        }
    ],
    "tags": null
}
`)
	// language=json
	event2 := []byte(`
{
    "timestamp": 0,
    "namespace": "",
    "path_id": "",
    "protocol": "UDP",
    "source": {
        "hostname": "abc",
        "via": null,
        "network_id": ""
    },
    "destination": {
        "hostname": "abc",
        "ip_address": "127.0.0.4",
        "port": 80
    },
    "hops": [
        {
            "ttl": 0,
            "ip_address": "1.1.1.3",
            "hostname": "hop_1",
            "rtt": 0,
            "success": false
        },
        {
            "ttl": 0,
            "ip_address": "1.1.1.4",
            "hostname": "hop_2",
            "rtt": 0,
            "success": false
        }
    ],
    "tags": null
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
			Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		},
		{
			Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		},
	}
	npCollector.ScheduleConns(conns)

	waitForProcessedPathtests(npCollector, 5*time.Second, 2)

	// THEN
	calls := stats.GaugeCalls
	tags := []string{
		"collector:network_path_collector",
		"destination_hostname:abc",
		"destination_port:80",
		"protocol:udp",
	}
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.path.monitored", Value: 1, Tags: tags, Rate: 1})

	assert.Equal(t, uint64(2), npCollector.processedTracerouteCount.Load())
	assert.Equal(t, uint64(2), npCollector.receivedPathtestCount.Load())

	app.RequireStop()
}

func Test_NpCollector_ScheduleConns_ScheduleDurationMetric(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	_, npCollector := newTestNpCollector(t, agentConfigs)

	stats := &teststatsd.Client{}
	npCollector.statsdClient = stats
	npCollector.metricSender = metricsender.NewMetricSenderStatsd(stats)

	conns := []*model.Connection{
		{
			Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		},
		{
			Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		},
	}
	timeNowCounter := 0
	npCollector.TimeNowFn = func() time.Time {
		now := MockTimeNow().Add(time.Duration(timeNowCounter) * time.Minute)
		timeNowCounter++
		return now
	}

	// WHEN
	npCollector.ScheduleConns(conns)

	// THEN
	calls := stats.GaugeCalls
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.schedule_duration", Value: 60.0, Tags: nil, Rate: 1})
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

	_, npCollector := newTestNpCollector(t, agentConfigs)

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 4, npCollector.workers)
	assert.Equal(t, 1000, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 1000, cap(npCollector.pathtestProcessingChan))
}

func Test_newNpCollectorImpl_overrideConfigs(t *testing.T) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              2,
		"network_path.collector.input_chan_size":      300,
		"network_path.collector.processing_chan_size": 400,
	}

	_, npCollector := newTestNpCollector(t, agentConfigs)

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 2, npCollector.workers)
	assert.Equal(t, 300, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 400, cap(npCollector.pathtestProcessingChan))
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
			name:         "one outgoing conn",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
		},
		{
			name:         "only non-outgoing conns",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
			},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:         "ignore non-outgoing conn",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
		},
		{
			name:         "no input chan",
			agentConfigs: defaultagentConfigs,
			noInputChan:  true,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
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
			conns:             createConns(10),
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"Error scheduling pathtests: collector input channel is full", 9},
			},
		},
		{
			name:         "only ipv4 supported",
			agentConfigs: defaultagentConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "::1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Family:    model.ConnectionFamily_v6,
				},
				{
					Laddr:     &model.Addr{Ip: "::1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
					Family:    model.ConnectionFamily_v6,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
			expectedLogs: []logCount{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, npCollector := newTestNpCollector(t, tt.agentConfigs)
			if tt.noInputChan {
				npCollector.pathtestInputChan = nil
			}

			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			stats := &teststatsd.Client{}
			npCollector.statsdClient = stats

			npCollector.ScheduleConns(tt.conns)

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
				if call.Name == "datadog.network_path.collector.schedule_duration" {
					scheduleDurationMetric = call
				}
			}
			assert.Less(t, scheduleDurationMetric.Value, float64(5)) // we can't easily assert precise value, hence we are only asserting that it's a low value e.g. 5 seconds
			scheduleDurationMetric.Value = 0                         // We need to reset the metric value to ease testing time duration
			assert.Equal(t, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.schedule_duration", Value: 0, Tags: nil, Rate: 1}, scheduleDurationMetric)

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

	_, npCollector := newTestNpCollector(t, agentConfigs)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	stopped := make(chan bool, 1)
	go func() {
		npCollector.startWorker(42)
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
				"datadog.network_path.collector.flush_interval",
			},
			expectedMetrics: []teststatsd.MetricsArgs{
				{Name: "datadog.network_path.collector.flush_duration", Value: 0.5, Tags: []string{}, Rate: 1},
			},
		},
		{
			name:               "with last flush time",
			flushStartTime:     MockTimeNow(),
			flushEndTime:       MockTimeNow().Add(500 * time.Millisecond),
			lastFlushTime:      MockTimeNow().Add(-2 * time.Minute),
			notExpectedMetrics: []string{},
			expectedMetrics: []teststatsd.MetricsArgs{
				{Name: "datadog.network_path.collector.flush_duration", Value: 0.5, Tags: []string{}, Rate: 1},
				{Name: "datadog.network_path.collector.flush_interval", Value: (2 * time.Minute).Seconds(), Tags: []string{}, Rate: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN
			agentConfigs := map[string]any{
				"network_path.connections_monitoring.enabled": true,
			}
			_, npCollector := newTestNpCollector(t, agentConfigs)

			stats := &teststatsd.Client{}
			npCollector.statsdClient = stats
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
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              6,
	}
	_, npCollector := newTestNpCollector(t, agentConfigs)

	stats := &teststatsd.Client{}
	npCollector.statsdClient = stats
	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host1", Port: 53})
	npCollector.pathtestStore.Add(&common.Pathtest{Hostname: "host2", Port: 53})

	// WHEN
	npCollector.flush()

	// THEN
	calls := stats.GaugeCalls
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.workers", Value: 6, Tags: []string{}, Rate: 1})
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.pathtest_store_size", Value: 2, Tags: []string{}, Rate: 1})
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.pathtest_flushed_count", Value: 2, Tags: []string{}, Rate: 1})

	assert.Equal(t, 2, len(npCollector.pathtestProcessingChan))
}

func Test_npCollectorImpl_sendTelemetry(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              6,
	}
	_, npCollector := newTestNpCollector(t, agentConfigs)

	stats := &teststatsd.Client{}
	npCollector.statsdClient = stats
	npCollector.metricSender = metricsender.NewMetricSenderStatsd(stats)
	path := payload.NetworkPath{
		Source:      payload.NetworkPathSource{Hostname: "abc"},
		Destination: payload.NetworkPathDestination{Hostname: "abc", IPAddress: "127.0.0.2", Port: 80},
		Hops: []payload.NetworkPathHop{
			{Hostname: "hop_1", IPAddress: "1.1.1.1"},
			{Hostname: "hop_2", IPAddress: "1.1.1.2"},
		},
	}
	ptestCtx := &pathteststore.PathtestContext{
		Pathtest: &common.Pathtest{Hostname: "127.0.0.2", Port: 80},
	}
	ptestCtx.SetLastFlushInterval(2 * time.Minute)
	npCollector.TimeNowFn = MockTimeNow
	checkStartTime := MockTimeNow().Add(-3 * time.Second)

	// WHEN
	npCollector.sendTelemetry(path, checkStartTime, ptestCtx)

	// THEN
	calls := stats.GaugeCalls
	tags := []string{
		"collector:network_path_collector",
		"destination_hostname:abc",
		"destination_port:80",
		"protocol:udp",
	}
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.check_duration", Value: 3, Tags: tags, Rate: 1})
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.check_interval", Value: (2 * time.Minute).Seconds(), Tags: tags, Rate: 1})
}
