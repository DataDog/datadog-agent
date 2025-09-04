// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package runner

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-traceroute/result"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestGetPorts(t *testing.T) {
	destPort, sourcePort, useSourcePort := getPorts(0)
	assert.GreaterOrEqual(t, destPort, uint16(DefaultDestPort))
	assert.GreaterOrEqual(t, sourcePort, uint16(DefaultSourcePort))
	assert.False(t, useSourcePort)

	destPort, sourcePort, useSourcePort = getPorts(80)
	assert.Equal(t, destPort, uint16(80))
	assert.GreaterOrEqual(t, sourcePort, uint16(DefaultSourcePort))
	assert.True(t, useSourcePort)
}

func TestProcessResults(t *testing.T) {
	runner := &Runner{}
	tts := []struct {
		description      string
		inputResults     *result.Results
		protocol         payload.Protocol
		hname            string
		destinationHost  string
		useGatewayLookup bool
		expected         payload.NetworkPath
		errMsg           string
	}{
		{
			description:  "nil results should return an empty result",
			inputResults: nil,
			errMsg:       "",
		},
		{
			description:      "test all fields",
			useGatewayLookup: false,
			protocol:         payload.ProtocolUDP,
			hname:            "test-hostname",
			destinationHost:  "test-destination-hostname",
			inputResults: &result.Results{
				Params: result.Params{
					Port: 33434,
				},
				Traceroute: result.Traceroute{
					Runs: []result.TracerouteRun{
						{
							RunID: "aa-bb-cc",
							Source: result.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: result.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      33434, // computer port or Boca Raton, FL?
							},
							Hops: []*result.TracerouteHop{
								{
									IPAddress: net.ParseIP("10.0.0.1"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.001, // seconds
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.003512345, // seconds
								},
							},
						},
					},
				},
				E2eProbe: result.E2eProbe{
					RTTs:                 []float64{0.100, 0.200},
					PacketsSent:          10,
					PacketsReceived:      5,
					PacketLossPercentage: 0.5,
					Jitter:               10,
					RTT: result.E2eProbeRTT{
						Avg: 15,
						Min: 10,
						Max: 20,
					},
				},
			},
			expected: payload.NetworkPath{
				AgentVersion: version.AgentVersion,
				Protocol:     payload.ProtocolUDP,
				Source: payload.NetworkPathSource{
					Hostname: "test-hostname",
				},
				Destination: payload.NetworkPathDestination{
					Hostname:  "test-destination-hostname",
					IPAddress: "8.8.8.8",
					Port:      33434,
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
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // seconds
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.003512345, // seconds
								},
							},
						},
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
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       0.001,
						Reachable: true,
					},
					{
						TTL:       2,
						IPAddress: "unknown_hop_2",
						Hostname:  "unknown_hop_2",
						RTT:       0,
						Reachable: false,
					},
					{
						TTL:       3,
						IPAddress: "172.0.0.255",
						Hostname:  "172.0.0.255",
						RTT:       0.003512345,
						Reachable: true,
					},
				},
			},
		},
		{
			description:      "successful processing no gateway lookup, did not reach target",
			useGatewayLookup: false,
			protocol:         payload.ProtocolUDP,
			hname:            "test-hostname",
			destinationHost:  "test-destination-hostname",
			inputResults: &result.Results{
				Params: result.Params{
					Port: 33434,
				},
				Traceroute: result.Traceroute{
					Runs: []result.TracerouteRun{
						{
							Source: result.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: result.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      33434, // computer port or Boca Raton, FL?
							},
							Hops: []*result.TracerouteHop{
								{
									IPAddress: net.ParseIP("10.0.0.1"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.001, // seconds
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.003512345, // seconds
								},
							},
						},
					},
				},
			},
			expected: payload.NetworkPath{
				AgentVersion: version.AgentVersion,
				Protocol:     payload.ProtocolUDP,
				Source: payload.NetworkPathSource{
					Hostname: "test-hostname",
				},
				Destination: payload.NetworkPathDestination{
					Hostname:  "test-destination-hostname",
					IPAddress: "8.8.8.8",
					Port:      33434,
				},
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
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
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // seconds
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.003512345, // seconds
								},
							},
						},
					},
				},
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       0.001,
						Reachable: true,
					},
					{
						TTL:       2,
						IPAddress: "unknown_hop_2",
						Hostname:  "unknown_hop_2",
						RTT:       0,
						Reachable: false,
					},
					{
						TTL:       3,
						IPAddress: "172.0.0.255",
						Hostname:  "172.0.0.255",
						RTT:       0.003512345,
						Reachable: true,
					},
				},
			},
		},
		{
			description:      "successful processing with gateway lookup, did not reach target",
			useGatewayLookup: true,
			protocol:         payload.ProtocolTCP,
			hname:            "test-hostname",
			destinationHost:  "test-destination-hostname",
			inputResults: &result.Results{
				Params: result.Params{
					Port: 443,
				},
				Traceroute: result.Traceroute{
					Runs: []result.TracerouteRun{
						{
							Source: result.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: result.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      443, // computer port or Boca Raton, FL?
							},
							Hops: []*result.TracerouteHop{
								{
									IPAddress: net.ParseIP("10.0.0.1"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.001, // 1ms
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.04, // 40ms
								},
							},
						},
					},
				},
			},
			expected: payload.NetworkPath{
				AgentVersion: version.AgentVersion,
				Protocol:     payload.ProtocolTCP,
				Source: payload.NetworkPathSource{
					Hostname: "test-hostname",
					Via: &network.Via{
						Subnet: network.Subnet{
							Alias: "test-subnet",
						},
					},
				},
				Destination: payload.NetworkPathDestination{
					Hostname:  "test-destination-hostname",
					IPAddress: "8.8.8.8",
					Port:      443,
				},
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
							Source: payload.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: payload.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      443, // computer port or Boca Raton, FL?
							},
							Hops: []payload.TracerouteHop{
								{
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // 1ms
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.04, // 40ms
								},
							},
						},
					},
				},
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       0.001,
						Reachable: true,
					},
					{
						TTL:       2,
						IPAddress: "unknown_hop_2",
						Hostname:  "unknown_hop_2",
						RTT:       0,
						Reachable: false,
					},
					{
						TTL:       3,
						IPAddress: "172.0.0.255",
						Hostname:  "172.0.0.255",
						RTT:       0.040,
						Reachable: true,
					},
				},
			},
		},
		{
			description:      "successful processing with gateway lookup, reached target",
			useGatewayLookup: true,
			protocol:         payload.ProtocolUDP,
			hname:            "test-hostname",
			destinationHost:  "test-destination-hostname",
			inputResults: &result.Results{
				Params: result.Params{
					Port: 33434,
				},
				Traceroute: result.Traceroute{
					Runs: []result.TracerouteRun{
						{
							Source: result.TracerouteSource{
								IPAddress: net.ParseIP("10.0.0.5"),
								Port:      12345,
							},
							Destination: result.TracerouteDestination{
								IPAddress: net.ParseIP("8.8.8.8"),
								Port:      33434, // computer port or Boca Raton, FL?
							},
							Hops: []*result.TracerouteHop{
								{
									IPAddress: net.ParseIP("10.0.0.1"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.001, // 1ms
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									ICMPType:  11,
									ICMPCode:  0,
									RTT:       0.08, // 80ms
								},
								{
									IPAddress: net.ParseIP("8.8.8.8"),
									Port:      443,
									RTT:       0.120,
								},
							},
						},
					},
				},
			},
			expected: payload.NetworkPath{
				AgentVersion: version.AgentVersion,
				Protocol:     payload.ProtocolUDP,
				Source: payload.NetworkPathSource{
					Hostname: "test-hostname",
					Via: &network.Via{
						Subnet: network.Subnet{
							Alias: "test-subnet",
						},
					},
				},
				Destination: payload.NetworkPathDestination{
					Hostname:  "test-destination-hostname",
					IPAddress: "8.8.8.8",
					Port:      33434,
				},
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
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
									IPAddress: net.ParseIP("10.0.0.1"),
									RTT:       0.001, // 1ms
								},
								{
									IPAddress: net.IP{},
								},
								{
									IPAddress: net.ParseIP("172.0.0.255"),
									RTT:       0.08, // 80ms
								},
								{
									IPAddress: net.ParseIP("8.8.8.8"),
									RTT:       0.120,
								},
							},
						},
					},
				},
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       0.001,
						Reachable: true,
					},
					{
						TTL:       2,
						IPAddress: "unknown_hop_2",
						Hostname:  "unknown_hop_2",
						RTT:       0,
						Reachable: false,
					},
					{
						TTL:       3,
						IPAddress: "172.0.0.255",
						Hostname:  "172.0.0.255",
						RTT:       0.08,
						Reachable: true,
					},
					{
						TTL:       4,
						IPAddress: "8.8.8.8",
						Hostname:  "8.8.8.8",
						RTT:       0.12,
						Reachable: true,
					},
				},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			// if gateway lookup is used, we need to mock the gateway lookup
			if test.useGatewayLookup {
				controller := gomock.NewController(t)
				defer controller.Finish()
				mockGatewayLookup := network.NewMockGatewayLookup(controller)
				runner.gatewayLookup = mockGatewayLookup
				runner.nsIno = 123
				mockGatewayLookup.EXPECT().LookupWithIPs(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&network.Via{
						Subnet: network.Subnet{
							Alias: "test-subnet",
						},
					},
				)
			}
			dstPort := uint16(0)
			if test.inputResults != nil {
				dstPort = uint16(test.inputResults.Params.Port)
			}
			actual, err := runner.processResults(test.inputResults, test.protocol, test.hname, test.destinationHost, dstPort)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errMsg)
				assert.Empty(t, actual)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, actual)
			diff := cmp.Diff(test.expected, actual,
				cmpopts.IgnoreFields(payload.NetworkPath{}, "Timestamp"),
				cmpopts.IgnoreFields(payload.NetworkPath{}, "PathtraceID"),
			)
			assert.Empty(t, diff)
		})
	}
}
