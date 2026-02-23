// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/netipx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/connfilter"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
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
		"network_path.collector.filters":              []map[string]any{},
	}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, nil)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
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
	_ = w.Flush()
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

type tracerouteRunner struct {
	fn func(context.Context, config.Config) (payload.NetworkPath, error)
}

func (t *tracerouteRunner) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	return t.fn(ctx, cfg)
}

func Test_NpCollector_runningAndProcessing(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled":      true,
		"network_path.collector.flush_interval":            "1s",
		"network_path.collector.monitor_ip_without_domain": true,
		"network_devices.namespace":                        "my-ns1",
		"network_path.collector.filters":                   []map[string]any{},
	}
	tr := &tracerouteRunner{func(_ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
		var p payload.NetworkPath
		if cfg.DestHostname == "10.0.0.2" {
			p = payload.NetworkPath{
				AgentVersion: "1.0.42",
				Protocol:     payload.ProtocolUDP,
				Source: payload.NetworkPathSource{
					Hostname:    "test-hostname",
					Name:        "test-hostname",
					DisplayName: "test-hostname",
				},
				Destination: payload.NetworkPathDestination{
					Hostname: "10.0.0.2",
					Port:     33434,
				},
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
							RunID: "aa-bb-cc",
							Source: payload.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: payload.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      33434, // computer port or Boca Raton, FL?
							},
							Hops: []payload.TracerouteHop{
								{
									TTL:       1,
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // seconds
								},
								{
									TTL:       2,
									IPAddress: net.IP{},
								},
								{
									TTL:       3,
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.003512345, // seconds
								},
							},
						},
					},
					HopCount: payload.HopCountStats{
						Avg: 10,
						Min: 5,
						Max: 15,
					},
				},
				E2eProbe: payload.E2eProbe{
					RTTs:                 []float64{0.100, 0.200},
					PacketsSent:          10,
					PacketsReceived:      5,
					PacketLossPercentage: 0.5,
					Jitter:               10,
					RTT: payload.E2eProbeRttLatency{
						Avg: 15,
						Min: 10,
						Max: 20,
					},
				},
			}
		}
		if cfg.DestHostname == "10.0.0.4" {
			p = payload.NetworkPath{
				AgentVersion: "1.0.42",
				Protocol:     payload.ProtocolUDP,
				Source: payload.NetworkPathSource{
					Hostname:    "test-hostname",
					Name:        "test-hostname",
					DisplayName: "test-hostname",
				},
				Destination: payload.NetworkPathDestination{
					Hostname: "10.0.0.4",
					Port:     33434,
				},
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
							RunID: "aa-bb-cc",
							Source: payload.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: payload.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      33434, // computer port or Boca Raton, FL?
							},
							Hops: []payload.TracerouteHop{
								{
									TTL:       1,
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // seconds
								},
								{
									TTL:       2,
									IPAddress: net.IP{},
								},
								{
									TTL:       3,
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.003512345, // seconds
								},
							},
						},
					},
					HopCount: payload.HopCountStats{
						Avg: 10,
						Min: 5,
						Max: 15,
					},
				},
				E2eProbe: payload.E2eProbe{
					RTTs:                 []float64{0.100, 0.200},
					PacketsSent:          10,
					PacketsReceived:      5,
					PacketLossPercentage: 0.5,
					Jitter:               10,
					RTT: payload.E2eProbeRttLatency{
						Avg: 15,
						Min: 10,
						Max: 20,
					},
				},
			}
		}
		return p, nil
	}}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, tr)

	mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))
	npCollector.epForwarder = mockEpForwarder

	app.RequireStart()

	assert.True(t, npCollector.running)

	// EXPECT
	// language=json
	event1 := []byte(`
{
    "timestamp": 0,
    "agent_version": "1.0.42",
    "namespace": "my-ns1",
    "test_config_id": "",
    "test_result_id": "",
    "test_run_id": "",
    "origin": "network_traffic",
    "test_run_type": "dynamic",
    "source_product": "network_path",
    "collector_type": "agent",
    "protocol": "UDP",
    "source": {
        "name": "test-hostname",
        "display_name": "test-hostname",
        "hostname": "test-hostname",
        "container_id": "testId2"
    },
    "destination": {
        "hostname": "10.0.0.4",
        "port": 33434
    },
    "traceroute": {
        "runs": [
            {
                "run_id": "aa-bb-cc",
                "source": {
                    "ip_address": "10.0.0.5",
                    "port": 12345
                },
                "destination": {
                    "ip_address": "8.8.8.8",
                    "port": 33434
                },
                "hops": [
                    {
                        "ttl": 1,
                        "ip_address": "10.0.0.1",
                        "rtt": 0.001,
                        "reachable": false
                    },
                    {
                        "ttl": 2,
                        "ip_address": "",
                        "reachable": false
                    },
                    {
                        "ttl": 3,
                        "ip_address": "172.0.0.255",
                        "rtt": 0.003512345,
                        "reachable": false
                    }
                ]
            }
        ],
        "hop_count": {
            "avg": 10,
            "min": 5,
            "max": 15
        }
    },
    "e2e_probe": {
        "rtts": [
            0.1,
            0.2
        ],
        "packets_sent": 10,
        "packets_received": 5,
        "packet_loss_percentage": 0.5,
        "jitter": 10,
        "rtt": {
            "avg": 15,
            "min": 10,
            "max": 20
        }
    }
}
`)
	// language=json
	event2 := []byte(`
{
    "timestamp": 0,
    "agent_version": "1.0.42",
    "namespace": "my-ns1",
    "test_config_id": "",
    "test_result_id": "",
    "test_run_id": "",
    "origin": "network_traffic",
    "test_run_type": "dynamic",
    "source_product": "network_path",
    "collector_type": "agent",
    "protocol": "UDP",
    "source": {
        "name": "test-hostname",
        "display_name": "test-hostname",
        "hostname": "test-hostname",
        "container_id": "testId1"
    },
    "destination": {
        "hostname": "10.0.0.2",
        "port": 33434
    },
    "traceroute": {
        "runs": [
            {
                "run_id": "aa-bb-cc",
                "source": {
                    "ip_address": "10.0.0.5",
                    "port": 12345
                },
                "destination": {
                    "ip_address": "8.8.8.8",
                    "port": 33434
                },
                "hops": [
                    {
                        "ttl": 1,
                        "ip_address": "10.0.0.1",
                        "rtt": 0.001,
                        "reachable": false
                    },
                    {
                        "ttl": 2,
                        "ip_address": "",
                        "reachable": false
                    },
                    {
                        "ttl": 3,
                        "ip_address": "172.0.0.255",
                        "rtt": 0.003512345,
                        "reachable": false
                    }
                ]
            }
        ],
        "hop_count": {
            "avg": 10,
            "min": 5,
            "max": 15
        }
    },
    "e2e_probe": {
        "rtts": [
            0.1,
            0.2
        ],
        "packets_sent": 10,
        "packets_received": 5,
        "packet_loss_percentage": 0.5,
        "jitter": 10,
        "rtt": {
            "avg": 15,
            "min": 10,
            "max": 20
        }
    }
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
	conns := []npmodel.NetworkPathConnection{
		{
			Source:            netip.MustParseAddrPort("10.0.0.1:30000"),
			SourceContainerID: "testId1",
			Dest:              netip.MustParseAddrPort("10.0.0.2:80"),
			Direction:         model.ConnectionDirection_outgoing,
			Type:              model.ConnectionType_tcp,
		},
		{
			Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
			SourceContainerID: "testId2",
			Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
			Direction:         model.ConnectionDirection_outgoing,
			Type:              model.ConnectionType_udp,
		},
	}
	npCollector.ScheduleNetworkPathTests(slices.Values(conns))

	waitForProcessedPathtests(npCollector, 5*time.Second, 2)

	// THEN
	assert.Equal(t, uint64(2), npCollector.processedTracerouteCount.Load())
	assert.Equal(t, uint64(2), npCollector.receivedPathtestCount.Load())

	app.RequireStop()
}

func Test_NpCollector_stopWithoutPanic(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled":      true,
		"network_path.collector.flush_interval":            "1s",
		"network_path.collector.workers":                   100,
		"network_path.collector.monitor_ip_without_domain": true,
		"network_devices.namespace":                        "my-ns1",
		"network_path.collector.filters":                   []map[string]any{},
	}
	tr := &tracerouteRunner{func(_ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond) // simulate slow processing time, to test for panic
		return payload.NetworkPath{
			TestRunID:   "test-run-id-111-" + cfg.DestHostname,
			Protocol:    cfg.Protocol,
			Source:      payload.NetworkPathSource{Hostname: "abc"},
			Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, Port: cfg.DestPort},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{
					{
						RunID: "aa-bb-cc",
						Source: payload.TracerouteSource{
							IPAddress: net.ParseIP("10.0.0.5"),
							Port:      12345,
						},
						Destination: payload.TracerouteDestination{
							IPAddress: net.ParseIP(cfg.DestHostname),
							Port:      33434, // computer port or Boca Raton, FL?
						},
						Hops: []payload.TracerouteHop{
							{ReverseDNS: []string{"hop_1"}, IPAddress: net.ParseIP("1.1.1.1")},
							{ReverseDNS: []string{"hop_2"}, IPAddress: net.ParseIP("1.1.1.2")},
						},
					},
				},
			},
		}, nil
	}}
	app, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, tr)

	app.RequireStart()

	assert.True(t, npCollector.running)

	// WHEN
	var conns []npmodel.NetworkPathConnection
	currentIP, _ := netip.ParseAddr("10.0.0.0")
	for i := 0; i < 1000; i++ {
		currentIP = netipx.AddrNext(currentIP)
		conns = append(conns, npmodel.NetworkPathConnection{
			Source:            netip.MustParseAddrPort("10.0.0.1:30000"),
			SourceContainerID: "testId1",
			Dest:              netip.MustParseAddrPort(currentIP.String() + ":80"),
			Direction:         model.ConnectionDirection_outgoing,
			Type:              model.ConnectionType_tcp,
		})
	}
	npCollector.ScheduleNetworkPathTests(slices.Values(conns))

	waitForProcessedPathtests(npCollector, 5*time.Second, 10)

	// THEN
	assert.GreaterOrEqual(t, int(npCollector.processedTracerouteCount.Load()), 10)

	// test that stop sequence won't trigger panic
	app.RequireStop()
}

func Test_NpCollector_ScheduleNetworkPathTests_ScheduleDurationMetric(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

	conns := []npmodel.NetworkPathConnection{
		{
			Source:            netip.MustParseAddrPort("10.0.0.1:30000"),
			SourceContainerID: "testId1",
			Dest:              netip.MustParseAddrPort("10.0.0.2:80"),
			Direction:         model.ConnectionDirection_outgoing,
			Type:              model.ConnectionType_tcp,
		},
		{
			Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
			SourceContainerID: "testId2",
			Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
			Direction:         model.ConnectionDirection_outgoing,
			Type:              model.ConnectionType_udp,
		},
	}
	timeNowCounter := 0
	npCollector.TimeNowFn = func() time.Time {
		now := MockTimeNow().Add(time.Duration(timeNowCounter) * time.Minute)
		timeNowCounter++
		return now
	}

	// WHEN
	npCollector.ScheduleNetworkPathTests(slices.Values(conns))

	// THEN
	calls := stats.GaugeCalls
	assert.Contains(t, calls, teststatsd.MetricsArgs{Name: "datadog.network_path.collector.schedule.duration", Value: 60.0, Tags: nil, Rate: 1})
}

func compactJSON(metadataEvent []byte) []byte {
	compactMetadataEvent := new(bytes.Buffer)
	_ = json.Compact(compactMetadataEvent, metadataEvent)
	return compactMetadataEvent.Bytes()
}

func Test_newNpCollectorImpl_defaultConfigs(t *testing.T) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, nil)

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 4, npCollector.workers)
	assert.Equal(t, 1000, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 1000, cap(npCollector.pathtestProcessingChan))
	assert.Equal(t, 1000, npCollector.collectorConfigs.storeConfig.ContextsLimit)
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

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, nil)

	assert.Equal(t, true, npCollector.collectorConfigs.networkPathCollectorEnabled())
	assert.Equal(t, 2, npCollector.workers)
	assert.Equal(t, 300, cap(npCollector.pathtestInputChan))
	assert.Equal(t, 400, cap(npCollector.pathtestProcessingChan))
	assert.Equal(t, 500, npCollector.collectorConfigs.storeConfig.ContextsLimit)
	assert.Equal(t, "ns1", npCollector.networkDevicesNamespace)
}

func Test_npCollectorImpl_ScheduleNetworkPathTests(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	defaultagentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.filters":              []map[string]any{},
	}
	monitorIPWithoutDomainConfigs := map[string]any{
		"network_path.connections_monitoring.enabled":      true,
		"network_path.collector.monitor_ip_without_domain": true,
		"network_path.collector.filters":                   []map[string]any{},
	}
	icmpModeConfigs := map[string]any{
		"network_path.connections_monitoring.enabled":      true,
		"network_path.collector.monitor_ip_without_domain": true,
		"network_path.collector.icmp_mode":                 "all",
		"network_path.collector.filters":                   []map[string]any{},
	}

	tests := []struct {
		name              string
		conns             []npmodel.NetworkPathConnection
		noInputChan       bool
		agentConfigs      map[string]any
		expectedPathtests []*common.Pathtest
		expectedLogs      []logCount
	}{
		{
			name:              "zero conn",
			agentConfigs:      defaultagentConfigs,
			conns:             nil,
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:         "one outgoing TCP conn",
			agentConfigs: monitorIPWithoutDomainConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1"},
			},
		},
		{
			name:         "one outgoing UDP conn",
			agentConfigs: monitorIPWithoutDomainConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.5:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.6:161"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_udp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.6", Port: uint16(0), Protocol: payload.ProtocolUDP, SourceContainerID: "testId1"},
			},
		},
		{
			name:         "only non-outgoing conns",
			agentConfigs: defaultagentConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.1:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.2:80"),
					Direction:         model.ConnectionDirection_incoming,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_incoming,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:         "ignore non-outgoing conn",
			agentConfigs: monitorIPWithoutDomainConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.1:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.2:80"),
					Direction:         model.ConnectionDirection_incoming,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId2"},
			},
		},
		{
			name:         "no input chan",
			agentConfigs: monitorIPWithoutDomainConfigs,
			noInputChan:  true,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"[ERROR] ScheduleNetworkPathTests-range1: Error scheduling pathtests: no input channel, please check that network path is enabled", 1},
			},
		},
		{
			name: "input chan is full",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled":      true,
				"network_path.collector.input_chan_size":           1,
				"network_path.collector.monitor_ip_without_domain": true,
				"network_path.collector.filters":                   []map[string]any{},
			},
			conns:             createConns(20),
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"collector input channel is full", 10},
			},
		},
		{
			name:         "only ipv4 supported",
			agentConfigs: monitorIPWithoutDomainConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("[::1]:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("[::1]:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Family:            model.ConnectionFamily_v6,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("[::1]:30000"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("[::1]:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Family:            model.ConnectionFamily_v6,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId3",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId3"},
			},
			expectedLogs: []logCount{},
		},
		{
			name:         "one outgoing TCP conn with known hostname (DNS)",
			agentConfigs: monitorIPWithoutDomainConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
					Domain:            "known-hostname",
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "known-hostname", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "known-hostname"}},
			},
		},
		{
			name:         "tcp connection in ICMP mode",
			agentConfigs: icmpModeConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.5:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.6:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.6", Protocol: payload.ProtocolICMP, SourceContainerID: "testId1"},
			},
		},
		{
			name:         "one outgoing TCP conn with domain hostname",
			agentConfigs: defaultagentConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.7:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("93.184.216.34:443"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
					Domain:            "example.com",
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "example.com", Port: uint16(443), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "example.com"}},
			},
		},
		{
			name:         "ipv6 conn with domain should be allowed",
			agentConfigs: defaultagentConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("[::1]:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("[2001:db8::1]:443"),
					Direction:         model.ConnectionDirection_outgoing,
					Family:            model.ConnectionFamily_v6,
					Type:              model.ConnectionType_tcp,
					Domain:            "ipv6-example.com",
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "ipv6-example.com", Port: uint16(443), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "ipv6-example.com"}},
			},
		},
		{
			name:         "multiple connections with and without domains",
			agentConfigs: defaultagentConfigs,
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.7:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("93.184.216.34:443"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
					Domain:            "valid.com",
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "valid.com", Port: uint16(443), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "valid.com"}},
			},
		},
		{
			name: "skip IP without domain when monitorIPWithoutDomain is false",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled":      true,
				"network_path.collector.monitor_ip_without_domain": false,
				"network_path.collector.filters":                   []map[string]any{},
			},
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name: "allow IP without domain when monitorIPWithoutDomain is true",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled":      true,
				"network_path.collector.monitor_ip_without_domain": true,
				"network_path.collector.filters":                   []map[string]any{},
			},
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.4", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId1"},
			},
		},
		{
			name: "exclude filter blocks matching domain",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled": true,
				"network_path.collector.filters": []map[string]any{
					{"match_domain": "blocked.com", "type": "exclude"},
				},
			},
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:443"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
					Domain:            "blocked.com",
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30001"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("10.0.0.5:443"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
					Domain:            "allowed.com",
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "allowed.com", Port: uint16(443), Protocol: payload.ProtocolTCP, SourceContainerID: "testId2",
					Metadata: common.PathtestMetadata{ReverseDNSHostname: "allowed.com"}},
			},
		},
		{
			name: "exclude filter blocks matching IP",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled":      true,
				"network_path.collector.monitor_ip_without_domain": true,
				"network_path.collector.filters": []map[string]any{
					{
						"type":     "exclude",
						"match_ip": "10.0.0.4",
					},
				},
			},
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.0.4:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30001"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("10.0.0.5:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.0.5", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId2"},
			},
		},
		{
			name: "exclude filter blocks matching CIDR",
			agentConfigs: map[string]any{
				"network_path.connections_monitoring.enabled":      true,
				"network_path.collector.monitor_ip_without_domain": true,
				"network_path.collector.filters": []map[string]any{
					{
						"type":     "exclude",
						"match_ip": "10.0.1.0/24",
					},
				},
			},
			conns: []npmodel.NetworkPathConnection{
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30000"),
					SourceContainerID: "testId1",
					Dest:              netip.MustParseAddrPort("10.0.1.100:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
				{
					Source:            netip.MustParseAddrPort("10.0.0.3:30001"),
					SourceContainerID: "testId2",
					Dest:              netip.MustParseAddrPort("10.0.2.100:80"),
					Direction:         model.ConnectionDirection_outgoing,
					Type:              model.ConnectionType_tcp,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "10.0.2.100", Port: uint16(80), Protocol: payload.ProtocolTCP, SourceContainerID: "testId2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, tt.agentConfigs, stats, nil)
			if tt.noInputChan {
				npCollector.pathtestInputChan = nil
			}

			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			npCollector.ScheduleNetworkPathTests(slices.Values(tt.conns))

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
			_ = w.Flush()
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

	_, npCollector := newTestNpCollector(t, agentConfigs, &teststatsd.Client{}, nil)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
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
	_ = w.Flush()
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
			_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

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
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)
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
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)
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

func Benchmark_npCollectorImpl_ScheduleNetworkPathTests(b *testing.B) {
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.workers":              50,
	}

	file, err := os.OpenFile("benchmark.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	assert.Nil(b, err)
	defer file.Close()
	w := bufio.NewWriter(file)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
	assert.Nil(b, err)
	utillog.SetupLogger(l, "debug")
	defer func() { _ = w.Flush() }()

	app, npCollector := newTestNpCollector(b, agentConfigs, &teststatsd.Client{}, nil)

	// TEST START
	app.RequireStart()
	assert.True(b, npCollector.running)

	// Generate 50 random connections
	connections := createBenchmarkConns(500, 100)

	b.ResetTimer() // Reset timer after setup

	for i := 0; i < b.N; i++ {
		// add line to avoid linter error
		_ = i
		npCollector.ScheduleNetworkPathTests(slices.Values(connections))

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
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

	// WHEN
	// Destination, hop 1, hop 3, hop 4 are private IPs, hop 2 is a public IP
	path := payload.NetworkPath{
		Destination: payload.NetworkPathDestination{Hostname: "dest-hostname"},
		Traceroute: payload.Traceroute{
			Runs: []payload.TracerouteRun{
				{
					RunID: "aa-bb-cc",
					Source: payload.TracerouteSource{
						IPAddress: net.ParseIP("10.0.0.5"),
						Port:      12345,
					},
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.41"),
						Port:      33434, // computer port or Boca Raton, FL?
					},
					Hops: []payload.TracerouteHop{
						{IPAddress: net.ParseIP("10.0.0.1"), Reachable: true, ReverseDNS: []string{"hop1"}},
						{IPAddress: net.ParseIP("1.1.1.1"), Reachable: true, ReverseDNS: []string{"hop2"}},
						{IPAddress: net.ParseIP("10.0.0.100"), Reachable: true, ReverseDNS: []string{"hop3"}},
						{IPAddress: net.ParseIP("10.0.0.41"), Reachable: true, ReverseDNS: []string{"dest-hostname"}},
					},
				}, {
					RunID: "11-22-33",
					Source: payload.TracerouteSource{
						IPAddress: net.ParseIP("10.0.0.5"),
						Port:      12345,
					},
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.41"),
						Port:      33434, // computer port or Boca Raton, FL?
					},
					Hops: []payload.TracerouteHop{
						{IPAddress: net.ParseIP("10.0.0.100"), Reachable: true, ReverseDNS: []string{"hop1"}},
						{IPAddress: net.ParseIP("10.0.0.101"), Reachable: true, ReverseDNS: []string{"hop1"}},
					},
				},
			},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")
	// THEN
	trRun := path.Traceroute.Runs[0]
	trRun2 := path.Traceroute.Runs[1]
	assert.Equal(t, []string{"hostname-10.0.0.41"}, trRun.Destination.ReverseDNS) // private IP should be resolved
	assert.Equal(t, []string{"hostname-10.0.0.1"}, trRun.Hops[0].ReverseDNS)
	assert.Equal(t, []string{"hop2"}, trRun.Hops[1].ReverseDNS) // public IP should fall back to hostname from traceroute
	assert.Equal(t, []string{"hostname-10.0.0.100"}, trRun.Hops[2].ReverseDNS)
	assert.Equal(t, []string{"hostname-10.0.0.41"}, trRun.Hops[3].ReverseDNS)
	assert.Equal(t, []string{"hostname-10.0.0.100"}, trRun2.Hops[0].ReverseDNS)
	assert.Equal(t, []string{"hostname-10.0.0.101"}, trRun2.Hops[1].ReverseDNS)

	// WHEN
	// hop 3 is a private IP, others are public IPs or unknown hops which should not be resolved
	path = payload.NetworkPath{
		Destination: payload.NetworkPathDestination{Hostname: "google.com"},
		Traceroute: payload.Traceroute{
			Runs: []payload.TracerouteRun{
				{
					RunID: "aa-bb-cc",
					Source: payload.TracerouteSource{
						IPAddress: net.ParseIP("10.0.0.5"),
						Port:      12345,
					},
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("8.8.8.8"),
						Port:      33434, // computer port or Boca Raton, FL?
					},
					Hops: []payload.TracerouteHop{
						{IPAddress: net.ParseIP("unknown-hop-1"), Reachable: false, ReverseDNS: []string{"hop1"}},
						{IPAddress: net.ParseIP("1.1.1.1"), Reachable: true, ReverseDNS: []string{"hop2"}},
						{IPAddress: net.ParseIP("10.0.0.100"), Reachable: true, ReverseDNS: []string{"hop3"}},
						{IPAddress: net.ParseIP("unknown-hop-4"), Reachable: false, ReverseDNS: []string{"hop4"}},
					},
				},
			},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")

	// THEN
	trRun = path.Traceroute.Runs[0]
	assert.Empty(t, trRun.Destination.ReverseDNS)
	assert.Equal(t, []string{"hop1"}, trRun.Hops[0].ReverseDNS)
	assert.Equal(t, []string{"hop2"}, trRun.Hops[1].ReverseDNS) // public IP should fall back to hostname from traceroute
	assert.Equal(t, []string{"hostname-10.0.0.100"}, trRun.Hops[2].ReverseDNS)
	assert.Equal(t, []string{"hop4"}, trRun.Hops[3].ReverseDNS)

	// GIVEN - no reverse DNS resolution
	agentConfigs = map[string]any{
		"network_path.connections_monitoring.enabled":           true,
		"network_path.collector.reverse_dns_enrichment.enabled": false,
	}
	_, npCollector = newTestNpCollector(t, agentConfigs, stats, nil)

	// WHEN
	// Destination, hop 1, hop 3, hop 4 are private IPs, hop 2 is a public IP
	path = payload.NetworkPath{
		Destination: payload.NetworkPathDestination{Hostname: "dest-hostname"},
		Traceroute: payload.Traceroute{
			Runs: []payload.TracerouteRun{
				{
					RunID: "aa-bb-cc",
					Source: payload.TracerouteSource{
						IPAddress: net.ParseIP("10.0.0.5"),
						Port:      12345,
					},
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.41"),
						Port:      33434, // computer port or Boca Raton, FL?
					},
					Hops: []payload.TracerouteHop{
						{IPAddress: net.ParseIP("10.0.0.1"), Reachable: true, ReverseDNS: []string{"hop1"}},
						{IPAddress: net.ParseIP("1.1.1.1"), Reachable: true, ReverseDNS: []string{"hop2"}},
						{IPAddress: net.ParseIP("10.0.0.100"), Reachable: true, ReverseDNS: []string{"hop3"}},
						{IPAddress: net.ParseIP("10.0.0.41"), Reachable: true, ReverseDNS: []string{"dest-hostname"}},
					},
				},
			},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "")

	// THEN - no resolution should happen
	trRun = path.Traceroute.Runs[0]
	assert.Empty(t, trRun.Destination.ReverseDNS)
	assert.Equal(t, []string{"hop1"}, trRun.Hops[0].ReverseDNS)
	assert.Equal(t, []string{"hop2"}, trRun.Hops[1].ReverseDNS)
	assert.Equal(t, []string{"hop3"}, trRun.Hops[2].ReverseDNS)
	assert.Equal(t, []string{"dest-hostname"}, trRun.Hops[3].ReverseDNS)
}

func Test_npCollectorImpl_enrichPathWithRDNSKnownHostName(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

	// WHEN
	path := payload.NetworkPath{
		Destination: payload.NetworkPathDestination{Hostname: "dest-hostname"},
		Traceroute: payload.Traceroute{
			Runs: []payload.TracerouteRun{
				{
					RunID: "aa-bb-cc",
					Source: payload.TracerouteSource{
						IPAddress: net.ParseIP("10.0.0.5"),
						Port:      12345,
					},
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.41"),
						Port:      33434, // computer port or Boca Raton, FL?
					},
				},
			},
		},
	}

	npCollector.enrichPathWithRDNS(&path, "known-dest-hostname")

	// THEN - destination hostname should resolve to known hostname
	assert.Equal(t, []string{"known-dest-hostname"}, path.Traceroute.Runs[0].Destination.ReverseDNS)
	assert.Empty(t, path.Traceroute.Runs[0].Hops)
}

func Test_npCollectorImpl_getReverseDNSResult(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_path.connections_monitoring.enabled": true,
	}
	stats := &teststatsd.Client{}
	_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

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
		name                   string
		conn                   npmodel.NetworkPathConnection
		vpcSubnets             []netip.Prefix
		shouldSchedule         bool
		subnetSkipped          bool
		sourceExcludes         map[string][]string
		destExcludes           map[string][]string
		filters                string
		monitorIPWithoutDomain bool
		connectionExcluded     bool
	}{
		{
			name: "should schedule",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			shouldSchedule: true,
		},
		{
			name: "should not schedule incoming conn",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_incoming,
				Family:    model.ConnectionFamily_v4,
				Domain:    "abc",
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule conn with none direction",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_none,
				Family:    model.ConnectionFamily_v4,
				Domain:    "abc",
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule ipv6 when there is no domain",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v6,
			},
			shouldSchedule: false,
		},
		{
			name: "should schedule for ipv6 cnn if domain is present",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v6,
				Domain:    "abc",
			},
			shouldSchedule: true,
		},
		{
			name: "should not schedule for loopback",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("127.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("127.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v4,
				Domain:    "abc",
				IntraHost: true, // loopback is always IntraHost
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule for intrahost",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v4,
				Domain:    "abc",
				IntraHost: true,
			},
			shouldSchedule: false,
		},
		// intra-vpc subnet skipping tests
		{
			name: "VPC: random subnet should schedule anyway",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			vpcSubnets:     []netip.Prefix{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "VPC: relevant subnet should skip",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("192.168.1.1:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			vpcSubnets:     []netip.Prefix{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		{
			name: "VPC: shouldn't skip local address even if the subnet matches",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("192.168.1.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.1:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			vpcSubnets:     []netip.Prefix{mustParseCIDR(t, "192.168.0.0/16")},
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "VPC: translated clusterIP should get matched",
			conn: npmodel.NetworkPathConnection{
				Source:         netip.MustParseAddrPort("192.168.1.1:30000"),
				Dest:           netip.MustParseAddrPort("192.168.1.1:80"),
				TranslatedDest: netip.MustParseAddrPort("10.1.2.3:80"),
				Direction:      model.ConnectionDirection_outgoing,
				Domain:         "abc",
			},
			vpcSubnets:     []netip.Prefix{mustParseCIDR(t, "10.0.0.0/8")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		{
			name: "VPC: source translation existing shouldn't break subnet check",
			conn: npmodel.NetworkPathConnection{
				Source:         netip.MustParseAddrPort("192.168.1.1:30000"),
				Dest:           netip.MustParseAddrPort("10.0.0.1:80"),
				TranslatedDest: netip.MustParseAddrPort("10.0.0.1:80"),
				Direction:      model.ConnectionDirection_outgoing,
				Domain:         "abc",
			},
			vpcSubnets:     []netip.Prefix{mustParseCIDR(t, "10.0.0.0/8")},
			shouldSchedule: false,
			subnetSkipped:  true,
		},
		// connection exclusion tests
		{
			name: "exclusion: block dest exactly",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			destExcludes: map[string][]string{
				"10.0.0.2": {"80"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest but different port",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			destExcludes: map[string][]string{
				"10.0.0.2": {"42"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
		{
			name: "exclusion: block source with port range",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			sourceExcludes: map[string][]string{
				"10.0.0.1": {"30000-30005"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest subnet",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			destExcludes: map[string][]string{
				"10.0.0.0/8": {"*"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: block dest subnet, no match",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("192.168.1.1:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			destExcludes: map[string][]string{
				"10.0.0.0/8": {"*"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
		{
			name: "exclusion: only UDP, matching case",
			conn: npmodel.NetworkPathConnection{
				Type:      model.ConnectionType_udp,
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:123"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			sourceExcludes: map[string][]string{
				"10.0.0.0/8": {"udp *"},
			},
			shouldSchedule:     false,
			connectionExcluded: true,
		},
		{
			name: "exclusion: only UDP, non-matching case",
			conn: npmodel.NetworkPathConnection{
				// (tcp is 0 so this doesn't actually do anything)
				Type:      model.ConnectionType_tcp,
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:123"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			sourceExcludes: map[string][]string{
				"10.0.0.0/8": {"udp *"},
			},
			shouldSchedule:     true,
			connectionExcluded: false,
		},
		{
			name: "exclusion: exclude system probe conn",
			conn: npmodel.NetworkPathConnection{
				Type:            model.ConnectionType_tcp,
				Source:          netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:            netip.MustParseAddrPort("10.0.0.2:123"),
				Direction:       model.ConnectionDirection_outgoing,
				Domain:          "abc",
				SystemProbeConn: true,
			},
			shouldSchedule: false,
		},
		{
			name: "FILTERS: excluded domain to test that filters works",
			filters: `
network_path:
  collector:
    filters:
      - match_domain: 'google.com'
        type: exclude
`,
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "google.com",
			},
			shouldSchedule: false,
		},
		{
			name: "FILTERS: include IP",
			filters: `
network_path:
  collector:
    filters:
      - match_ip: '10.10.10.10'
        type: include
`,
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.10.10.10:80"),
				Direction: model.ConnectionDirection_outgoing,
			},
			shouldSchedule: true,
		},
		{
			name: "FILTERS: test monitor all IPs without domain",
			//domain:                 "",
			monitorIPWithoutDomain: true,
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
			},
			shouldSchedule: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configs []connfilter.Config
			if tt.filters != "" {
				cfg := configComponent.NewMockFromYAML(t, tt.filters)
				err := structure.UnmarshalKey(cfg, "network_path.collector.filters", &configs)
				require.NoError(t, err)
			}
			// Ensure we have an empty slice (not nil) to override global defaults
			if configs == nil {
				configs = []connfilter.Config{}
			}
			agentConfigs := map[string]any{
				"network_path.connections_monitoring.enabled":         true,
				"network_path.collector.disable_intra_vpc_collection": true,
				"network_path.collector.source_excludes":              tt.sourceExcludes,
				"network_path.collector.dest_excludes":                tt.destExcludes,
				"network_path.collector.filters":                      configs,
				"network_path.collector.monitor_ip_without_domain":    tt.monitorIPWithoutDomain,
			}
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

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

func mustParseCIDR(t *testing.T, cidr string) netip.Prefix {
	ipNet, err := netip.ParsePrefix(cidr)
	assert.NoError(t, err)
	return ipNet
}

func Test_npCollectorImpl_shouldScheduleNetworkPathForConn_subnets(t *testing.T) {
	tests := []struct {
		name           string
		conn           npmodel.NetworkPathConnection
		vpcSubnets     []*net.IPNet
		shouldSchedule bool
		subnetSkipped  bool
	}{
		{
			name: "should schedule",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_outgoing,
				Domain:    "abc",
			},
			vpcSubnets:     nil,
			shouldSchedule: true,
			subnetSkipped:  false,
		},
		{
			name: "should not schedule incoming conn",
			conn: npmodel.NetworkPathConnection{
				Source:    netip.MustParseAddrPort("10.0.0.1:30000"),
				Dest:      netip.MustParseAddrPort("10.0.0.2:80"),
				Direction: model.ConnectionDirection_incoming,
				Family:    model.ConnectionFamily_v4,
				Domain:    "abc",
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
				"network_path.collector.filters":                      []map[string]any{},
			}
			stats := &teststatsd.Client{}
			_, npCollector := newTestNpCollector(t, agentConfigs, stats, nil)

			assert.Equal(t, tt.shouldSchedule, npCollector.shouldScheduleNetworkPathForConn(tt.conn, nil))

			if tt.subnetSkipped {
				require.Contains(t, stats.CountCalls, subnetSkippedStat)
			} else {
				require.NotContains(t, stats.CountCalls, subnetSkippedStat)
			}
		})
	}

}
