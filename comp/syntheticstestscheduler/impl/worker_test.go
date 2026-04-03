// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"crypto/rand"
	"errors"
	"io"
	"math"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/stretchr/testify/require"
)

func TestToNetpathConfig(t *testing.T) {
	udpPort := 53
	udpTTL := 32
	udpTimeout := 5
	src := "frontend"
	dst := "backend"

	tcpPort := 443
	tcpTTL := 64
	tcpTimeout := 10

	icmpTTL := 5
	icmpTimeout := 2

	tracerouteCount := 3
	probeCount := 50

	tests := []struct {
		name        string
		input       common.SyntheticsTestConfig
		expect      config.Config
		expectError bool
	}{
		{
			name: "UDP request",
			input: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Request: common.UDPConfigRequest{
						Host: "dns.example.com",
						Port: &udpPort,
						NetworkConfigRequest: common.NetworkConfigRequest{
							SourceService:      &src,
							DestinationService: &dst,
							MaxTTL:             &udpTTL,
							Timeout:            &udpTimeout,
							ProbeCount:         &probeCount,
							TracerouteCount:    &tracerouteCount,
						},
					},
				},
			},
			expect: config.Config{
				Protocol:           payload.ProtocolUDP,
				DestHostname:       "dns.example.com",
				DestPort:           uint16(udpPort),
				MaxTTL:             uint8(udpTTL),
				Timeout:            time.Duration(float64(udpTimeout) * 0.9 / float64(udpTTL) * float64(time.Second)),
				SourceService:      src,
				DestinationService: dst,
				ReverseDNS:         true,
				TracerouteQueries:  tracerouteCount,
				E2eQueries:         probeCount,
			},
			expectError: false,
		},
		{
			name: "TCP request",
			input: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Request: common.TCPConfigRequest{
						Host:      "web.example.com",
						Port:      &tcpPort,
						TCPMethod: payload.TCPConfigSYN,
						NetworkConfigRequest: common.NetworkConfigRequest{
							SourceService:      &src,
							DestinationService: &dst,
							MaxTTL:             &tcpTTL,
							Timeout:            &tcpTimeout,
							ProbeCount:         &probeCount,
							TracerouteCount:    &tracerouteCount,
						},
					},
				},
			},
			expect: config.Config{
				Protocol:           payload.ProtocolTCP,
				DestHostname:       "web.example.com",
				DestPort:           uint16(tcpPort),
				MaxTTL:             uint8(tcpTTL),
				Timeout:            time.Duration(float64(tcpTimeout) * 0.9 / float64(tcpTTL) * float64(time.Second)),
				TCPMethod:          payload.TCPConfigSYN,
				SourceService:      src,
				DestinationService: dst,
				ReverseDNS:         true,
				TracerouteQueries:  tracerouteCount,
				E2eQueries:         probeCount,
			},
			expectError: false,
		},
		{
			name: "ICMP request",
			input: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Request: common.ICMPConfigRequest{
						Host: "8.8.8.8",
						NetworkConfigRequest: common.NetworkConfigRequest{
							SourceService:      &src,
							DestinationService: &dst,
							MaxTTL:             &icmpTTL,
							Timeout:            &icmpTimeout,
							ProbeCount:         &probeCount,
							TracerouteCount:    &tracerouteCount,
						},
					},
				},
			},
			expect: config.Config{
				Protocol:           payload.ProtocolICMP,
				DestHostname:       "8.8.8.8",
				MaxTTL:             uint8(icmpTTL),
				Timeout:            time.Duration(float64(icmpTimeout) * 0.9 / float64(icmpTTL) * float64(time.Second)),
				SourceService:      src,
				DestinationService: dst,
				ReverseDNS:         true,
				TracerouteQueries:  tracerouteCount,
				E2eQueries:         probeCount,
			},
			expectError: false,
		},
		{
			name: "Unsupported subtype",
			input: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toNetpathConfig(tt.input)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expect, got)
		})
	}
}

func TestNetworkPathToTestResult(t *testing.T) {
	src := "frontend"
	dst := "backend"
	icmpTTL := 5
	icmpTimeout := 2

	now := time.Now()
	trCfg := config.Config{
		DestHostname: "example.com",
		DestPort:     80,
		MaxTTL:       30,
		Timeout:      2 * time.Second,
	}

	tests := []struct {
		name        string
		worker      workerResult
		expectFail  bool
		expectError bool
	}{
		{
			name: "success case",
			worker: workerResult{
				tracerouteResult: payload.NetworkPath{
					E2eProbe: payload.E2eProbe{
						PacketsSent:          10,
						PacketsReceived:      9,
						PacketLossPercentage: 10,
						Jitter:               []float64{5}[0],
						RTT: payload.E2eProbeRttLatency{
							Avg: 20, Min: 15, Max: 25,
						},
					},
					Traceroute: payload.Traceroute{
						HopCount: payload.HopCountStats{Avg: 5, Min: 4, Max: 6},
					},
				},
				tracerouteCfg: trCfg,
				testCfg: SyntheticsTestCtx{
					cfg: common.SyntheticsTestConfig{
						PublicID: "pub-123",
						Type:     "network",
						Version:  1,
						Config: struct {
							Assertions []common.Assertion   `json:"assertions"`
							Request    common.ConfigRequest `json:"request"`
						}{
							Request: common.ICMPConfigRequest{
								Host: "8.8.8.8",
								NetworkConfigRequest: common.NetworkConfigRequest{
									SourceService:      &src,
									DestinationService: &dst,
									MaxTTL:             &icmpTTL,
									Timeout:            &icmpTimeout,
								},
							},
						},
					},
				},
				triggeredAt: now.Add(-3 * time.Second),
				startedAt:   now.Add(-2 * time.Second),
				finishedAt:  now,
				duration:    2 * time.Second,
				hostname:    "agent-host",
			},
			expectFail:  false,
			expectError: false,
		},
		{
			name: "Jitter value is 0",
			worker: workerResult{
				tracerouteResult: payload.NetworkPath{
					E2eProbe: payload.E2eProbe{
						PacketsSent:          10,
						PacketsReceived:      9,
						PacketLossPercentage: 10,
						Jitter:               []float64{0}[0],
						RTT: payload.E2eProbeRttLatency{
							Avg: 20, Min: 15, Max: 25,
						},
					},
					Traceroute: payload.Traceroute{
						HopCount: payload.HopCountStats{Avg: 5, Min: 4, Max: 6},
					},
				},
				tracerouteCfg: trCfg,
				testCfg: SyntheticsTestCtx{
					cfg: common.SyntheticsTestConfig{
						PublicID: "pub-123",
						Type:     "network",
						Version:  1,
						Config: struct {
							Assertions []common.Assertion   `json:"assertions"`
							Request    common.ConfigRequest `json:"request"`
						}{
							Request: common.ICMPConfigRequest{
								Host: "8.8.8.8",
								NetworkConfigRequest: common.NetworkConfigRequest{
									SourceService:      &src,
									DestinationService: &dst,
									MaxTTL:             &icmpTTL,
									Timeout:            &icmpTimeout,
								},
							},
						},
					},
				},
				triggeredAt: now.Add(-3 * time.Second),
				startedAt:   now.Add(-2 * time.Second),
				finishedAt:  now,
				duration:    2 * time.Second,
				hostname:    "agent-host",
			},
			expectFail:  false,
			expectError: false,
		},
		{
			name: "100% packet loss",
			worker: workerResult{
				tracerouteResult: payload.NetworkPath{
					E2eProbe: payload.E2eProbe{
						PacketsSent:          0,
						PacketsReceived:      0,
						PacketLossPercentage: 1,
						Jitter:               []float64{0}[0],
						RTT: payload.E2eProbeRttLatency{
							Avg: 0, Min: 0, Max: 0,
						},
					},
					Traceroute: payload.Traceroute{
						HopCount: payload.HopCountStats{Avg: 5, Min: 4, Max: 6},
					},
				},
				tracerouteCfg: trCfg,
				testCfg: SyntheticsTestCtx{
					cfg: common.SyntheticsTestConfig{
						PublicID: "pub-123",
						Type:     "network",
						Version:  1,
						Config: struct {
							Assertions []common.Assertion   `json:"assertions"`
							Request    common.ConfigRequest `json:"request"`
						}{
							Request: common.ICMPConfigRequest{
								Host: "8.8.8.8",
								NetworkConfigRequest: common.NetworkConfigRequest{
									SourceService:      &src,
									DestinationService: &dst,
									MaxTTL:             &icmpTTL,
									Timeout:            &icmpTimeout,
								},
							},
						},
					},
				},
				triggeredAt: now.Add(-3 * time.Second),
				startedAt:   now.Add(-2 * time.Second),
				finishedAt:  now,
				duration:    2 * time.Second,
				hostname:    "agent-host",
			},
			expectFail:  true,
			expectError: false,
		},
		{
			name: "100% packet loss with assertion on it",
			worker: workerResult{
				tracerouteResult: payload.NetworkPath{
					E2eProbe: payload.E2eProbe{
						PacketsSent:          0,
						PacketsReceived:      0,
						PacketLossPercentage: 1,
						Jitter:               []float64{0}[0],
						RTT: payload.E2eProbeRttLatency{
							Avg: 0, Min: 0, Max: 0,
						},
					},
					Traceroute: payload.Traceroute{
						HopCount: payload.HopCountStats{Avg: 5, Min: 4, Max: 6},
					},
				},
				tracerouteCfg: trCfg,
				testCfg: SyntheticsTestCtx{
					cfg: common.SyntheticsTestConfig{
						PublicID: "pub-123",
						Type:     "network",
						Version:  1,
						Config: struct {
							Assertions []common.Assertion   `json:"assertions"`
							Request    common.ConfigRequest `json:"request"`
						}{
							Request: common.ICMPConfigRequest{
								Host: "8.8.8.8",
								NetworkConfigRequest: common.NetworkConfigRequest{
									SourceService:      &src,
									DestinationService: &dst,
									MaxTTL:             &icmpTTL,
									Timeout:            &icmpTimeout,
								},
							},
						},
					},
				},
				assertionResult: []common.AssertionResult{{
					Operator: common.OperatorIs,
					Type:     common.AssertionTypePacketLoss,
					Expected: "1",
					Valid:    true,
				}},
				triggeredAt: now.Add(-3 * time.Second),
				startedAt:   now.Add(-2 * time.Second),
				finishedAt:  now,
				duration:    2 * time.Second,
				hostname:    "agent-host",
			},
			expectFail:  false,
			expectError: false,
		},
		{
			name: "failure case",
			worker: workerResult{
				tracerouteResult: payload.NetworkPath{},
				tracerouteError:  errors.New("connection timeout"),
				tracerouteCfg:    trCfg,
				testCfg: SyntheticsTestCtx{
					cfg: common.SyntheticsTestConfig{
						PublicID: "pub-456",
						Type:     "network",
						Version:  1,
						Config: struct {
							Assertions []common.Assertion   `json:"assertions"`
							Request    common.ConfigRequest `json:"request"`
						}{
							Request: common.ICMPConfigRequest{
								Host: "8.8.8.8",
								NetworkConfigRequest: common.NetworkConfigRequest{
									SourceService:      &src,
									DestinationService: &dst,
									MaxTTL:             &icmpTTL,
									Timeout:            &icmpTimeout,
								},
							},
						},
					},
				},
				triggeredAt: now.Add(-4 * time.Second),
				startedAt:   now.Add(-3 * time.Second),
				finishedAt:  now,
				duration:    3 * time.Second,
				hostname:    "agent-host",
			},
			expectFail:  true,
			expectError: false,
		},
	}

	sched := &syntheticsTestScheduler{
		generateTestResultID: func(func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error) {
			return "test-result-id-123", nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sched.networkPathToTestResult(&tt.worker)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.worker.testCfg.cfg.PublicID, got.Test.ID)
			require.Equal(t, "test-result-id-123", got.Result.ID)
			require.Equal(t, tt.worker.testCfg.cfg.Config.Request.(common.ICMPConfigRequest).Host, got.Result.Config.Request.Host)
			require.Nil(t, got.Result.Config.Request.Port)
			require.NotNil(t, got.Result.Netpath.Destination.Port)

			require.Equal(t, payload.PathOriginSynthetics, got.Result.Netpath.Origin)
			require.Equal(t, payload.TestRunTypeScheduled, got.Result.Netpath.TestRunType)
			require.Equal(t, payload.SourceProductSynthetics, got.Result.Netpath.SourceProduct)
			require.Equal(t, payload.CollectorTypeAgent, got.Result.Netpath.CollectorType)

			if tt.worker.tracerouteResult.E2eProbe.RTT.Max == 0 {
				require.Nil(t, got.Result.Netstats.Latency)
				require.Nil(t, got.Result.Netstats.Jitter)
			} else {
				require.NotNil(t, got.Result.Netstats.Jitter)
			}

			if tt.expectFail {
				require.Equal(t, "failed", got.Result.Status)
				require.NotNil(t, got.Result.Failure)
			} else {
				require.Equal(t, "passed", got.Result.Status)
				require.Nil(t, got.Result.Failure)
			}
		})
	}
}

func TestNetworkPathToTestResult_UsesBackendResultID(t *testing.T) {
	src := "frontend"
	dst := "backend"
	icmpTTL := 5
	icmpTimeout := 2

	sched := &syntheticsTestScheduler{
		generateTestResultID: func(func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error) {
			return "generated-id", nil
		},
	}

	worker := workerResult{
		testCfg: SyntheticsTestCtx{
			cfg: common.SyntheticsTestConfig{
				PublicID: "pub-on-demand",
				ResultID: "backend-result-id",
				Type:     "network",
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Request: common.ICMPConfigRequest{
						Host: "8.8.8.8",
						NetworkConfigRequest: common.NetworkConfigRequest{
							SourceService:      &src,
							DestinationService: &dst,
							MaxTTL:             &icmpTTL,
							Timeout:            &icmpTimeout,
						},
					},
				},
			},
		},
		hostname: "agent-host",
	}

	got, err := sched.networkPathToTestResult(&worker)
	require.NoError(t, err)
	require.Equal(t, "backend-result-id", got.Result.ID)
}

func TestGenerateRandomStringUInt63(t *testing.T) {
	t.Run("success with mocked value", func(t *testing.T) {
		randIntFn := func(_ io.Reader, _ *big.Int) (*big.Int, error) {
			return big.NewInt(42), nil
		}
		got, err := generateRandomStringUInt63(randIntFn)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})

	t.Run("error path", func(t *testing.T) {
		randIntFn := func(_ io.Reader, _ *big.Int) (*big.Int, error) {
			return nil, errors.New("some errors")
		}

		got, err := generateRandomStringUInt63(randIntFn)
		require.Error(t, err)
		require.Empty(t, got)
	})

	t.Run("range check with real randomness", func(t *testing.T) {
		for i := 0; i < 10; i++ { // run multiple times
			got, err := generateRandomStringUInt63(rand.Int)
			require.NoError(t, err)

			val, err := strconv.ParseUint(got, 10, 64)
			require.NoError(t, err)

			// Assert it's within 0 <= val < 2^63
			require.Less(t, val, uint64(math.MaxInt64)+1)
		}
	})
}
