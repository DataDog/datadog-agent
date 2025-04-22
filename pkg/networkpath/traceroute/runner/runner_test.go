// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package runner

import (
	"fmt"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/sack"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		inputResults     *common.Results
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
			description:      "successful processing no gateway lookup, did not reach target",
			useGatewayLookup: false,
			protocol:         payload.ProtocolUDP,
			hname:            "test-hostname",
			destinationHost:  "test-destination-hostname",
			inputResults: &common.Results{
				Source:     net.ParseIP("10.0.0.5"),
				SourcePort: 12345,
				Target:     net.ParseIP("8.8.8.8"),
				DstPort:    33434, // computer port or Boca Raton, FL?
				Hops: []*common.Hop{
					{
						IP:       net.ParseIP("10.0.0.1"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      10000000, // 10ms
					},
					{
						IP: net.IP{},
					},
					{
						IP:       net.ParseIP("172.0.0.255"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      3512345, // 3.512ms
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
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       10,
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
						RTT:       3.512,
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
			inputResults: &common.Results{
				Source:     net.ParseIP("10.0.0.5"),
				SourcePort: 12345,
				Target:     net.ParseIP("8.8.8.8"),
				DstPort:    443, // computer port or Boca Raton, FL?
				Hops: []*common.Hop{
					{
						IP:       net.ParseIP("10.0.0.1"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      1000000, // 1ms
					},
					{
						IP: net.IP{},
					},
					{
						IP:       net.ParseIP("172.0.0.255"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      40000000, // 40ms
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
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       1,
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
						RTT:       40,
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
			inputResults: &common.Results{
				Source:     net.ParseIP("10.0.0.5"),
				SourcePort: 12345,
				Target:     net.ParseIP("8.8.8.8"),
				DstPort:    33434, // computer port or Boca Raton, FL?
				Hops: []*common.Hop{
					{
						IP:       net.ParseIP("10.0.0.1"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      1000000, // 1ms
					},
					{
						IP: net.IP{},
					},
					{
						IP:       net.ParseIP("172.0.0.255"),
						ICMPType: 11,
						ICMPCode: 0,
						RTT:      80000000, // 80ms
					},
					{
						IP:   net.ParseIP("8.8.8.8"),
						Port: 443,
						RTT:  120000000, // 120ms
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
				Hops: []payload.NetworkPathHop{
					{
						TTL:       1,
						IPAddress: "10.0.0.1",
						Hostname:  "10.0.0.1",
						RTT:       1,
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
						RTT:       80,
						Reachable: true,
					},
					{
						TTL:       4,
						IPAddress: "8.8.8.8",
						Hostname:  "8.8.8.8",
						RTT:       120,
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
			actual, err := runner.processResults(test.inputResults, test.protocol, test.hname, test.destinationHost)
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

func neverCalled(t *testing.T) tracerouteImpl {
	return func() (*common.Results, error) {
		t.Fatal("should not call this")
		return nil, fmt.Errorf("should not call this")
	}
}

func TestTCPFallback(t *testing.T) {
	dummySyn := &common.Results{}
	dummySack := &common.Results{}
	dummyErr := fmt.Errorf("test error")
	dummySackUnsupportedErr := &sack.NotSupportedError{
		Err: fmt.Errorf("dummy sack unsupported"),
	}

	t.Run("force SYN", func(t *testing.T) {
		doSyn := func() (*common.Results, error) {
			return dummySyn, nil
		}
		doSack := neverCalled(t)
		// success case
		results, err := performTCPFallback(payload.TCPConfigSYN, doSyn, doSack)
		require.NoError(t, err)
		require.Equal(t, dummySyn, results)

		doSyn = func() (*common.Results, error) {
			return nil, dummyErr
		}
		// error case
		results, err = performTCPFallback(payload.TCPConfigSYN, doSyn, doSack)
		require.Equal(t, dummyErr, err)
		require.Nil(t, results)
	})

	t.Run("force SACK", func(t *testing.T) {
		doSyn := neverCalled(t)
		doSack := func() (*common.Results, error) {
			return dummySack, nil
		}
		// success case
		results, err := performTCPFallback(payload.TCPConfigSACK, doSyn, doSack)
		require.NoError(t, err)
		require.Equal(t, dummySack, results)

		doSack = func() (*common.Results, error) {
			return nil, dummyErr
		}
		// error case
		results, err = performTCPFallback(payload.TCPConfigSACK, doSyn, doSack)
		require.Equal(t, dummyErr, err)
		require.Nil(t, results)
	})

	t.Run("prefer SACK - only running sack", func(t *testing.T) {
		doSyn := neverCalled(t)
		doSack := func() (*common.Results, error) {
			return dummySack, nil
		}
		// success case
		results, err := performTCPFallback(payload.TCPConfigPreferSACK, doSyn, doSack)
		require.NoError(t, err)
		require.Equal(t, dummySack, results)

		doSack = func() (*common.Results, error) {
			return nil, dummyErr
		}
		// error case (sack encounters a fatal error and does not fall back to SYN)
		results, err = performTCPFallback(payload.TCPConfigPreferSACK, doSyn, doSack)
		require.ErrorIs(t, err, dummyErr)
		require.Nil(t, results)
	})

	t.Run("prefer SACK - fallback case", func(t *testing.T) {
		doSyn := func() (*common.Results, error) {
			return dummySyn, nil
		}
		doSack := func() (*common.Results, error) {
			// cause a fallback because the target doesn't support SACK
			return nil, dummySackUnsupportedErr
		}
		// success case
		results, err := performTCPFallback(payload.TCPConfigPreferSACK, doSyn, doSack)
		require.NoError(t, err)
		require.Equal(t, dummySyn, results)

		doSyn = func() (*common.Results, error) {
			return nil, dummyErr
		}
		// error case
		results, err = performTCPFallback(payload.TCPConfigPreferSACK, doSyn, doSack)
		require.Equal(t, dummyErr, err)
		require.Nil(t, results)
	})
}
