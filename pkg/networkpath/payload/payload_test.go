// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package payload

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestICMPMode(t *testing.T) {
	require.True(t, ICMPModeNone.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeTCP.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeUDP.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolICMP))

	require.False(t, ICMPModeNone.ShouldUseICMP(ProtocolTCP))
	require.True(t, ICMPModeTCP.ShouldUseICMP(ProtocolTCP))
	require.False(t, ICMPModeUDP.ShouldUseICMP(ProtocolTCP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolTCP))

	require.False(t, ICMPModeNone.ShouldUseICMP(ProtocolUDP))
	require.False(t, ICMPModeTCP.ShouldUseICMP(ProtocolUDP))
	require.True(t, ICMPModeUDP.ShouldUseICMP(ProtocolUDP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolUDP))
}

func TestValidateNetworkPath(t *testing.T) {
	tests := []struct {
		name        string
		path        *NetworkPath
		expectError bool
		errorText   string
	}{
		{
			name: "all valid IP addresses",
			path: &NetworkPath{
				Destination: NetworkPathDestination{
					Hostname: "destination.com",
					Port:     443,
				},
				Traceroute: Traceroute{
					Runs: []TracerouteRun{
						{
							RunID: "runid0",
							Destination: TracerouteDestination{
								IPAddress: net.ParseIP("1.2.3.4"),
								Port:      443,
							},
						},
						{
							RunID: "runid1",
							Destination: TracerouteDestination{
								IPAddress: net.ParseIP("1.2.3.4"),
								Port:      443,
							},
						},
						{
							RunID: "runid2",
							Destination: TracerouteDestination{
								IPAddress: net.ParseIP("1.2.3.4"),
								Port:      443,
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "all invalid IP addresses",
			path: &NetworkPath{
				Destination: NetworkPathDestination{
					Hostname: "destination.com",
					Port:     443,
				},
				Traceroute: Traceroute{
					Runs: []TracerouteRun{
						{
							RunID: "runid0",
							Destination: TracerouteDestination{
								IPAddress: net.IP{},
								Port:      443,
							},
						},
						{
							RunID: "runid1",
							Destination: TracerouteDestination{
								IPAddress: net.IP{},
								Port:      443,
							},
						},
						{
							RunID: "runid2",
							Destination: TracerouteDestination{
								IPAddress: net.IP{},
								Port:      443,
							},
						},
					},
				},
			},
			expectError: true,
			errorText:   "invalid destination IP address for destination.com:443",
		},
		{
			name: "one invalid IP address",
			path: &NetworkPath{
				Destination: NetworkPathDestination{
					Hostname: "destination.com",
					Port:     443,
				},
				Traceroute: Traceroute{
					Runs: []TracerouteRun{
						{
							RunID: "runid0",
							Destination: TracerouteDestination{
								IPAddress: net.ParseIP("1.2.3.4"),
								Port:      443,
							},
						},
						{
							RunID: "runid1",
							Destination: TracerouteDestination{
								IPAddress: net.IP{},
								Port:      443,
							},
						},
						{
							RunID: "runid2",
							Destination: TracerouteDestination{
								IPAddress: net.ParseIP("1.2.3.4"),
								Port:      443,
							},
						},
					},
				},
			},
			expectError: true,
			errorText:   "invalid destination IP address for destination.com:443",
		},
		{
			name:        "nil path",
			path:        nil,
			expectError: true,
			errorText:   "invalid nil path",
		},
		{
			name: "empty runs",
			path: &NetworkPath{
				Destination: NetworkPathDestination{
					Hostname: "destination.com",
				},
				Traceroute: Traceroute{
					Runs: []TracerouteRun{},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkPath(tt.path)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorText != "" {
					require.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
