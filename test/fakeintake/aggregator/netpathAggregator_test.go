// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/netpath_bytes
var netpathData []byte

func TestNetpathAggregator(t *testing.T) {
	t.Run("ParseNetpathPayload should return empty Netpath array on empty data", func(t *testing.T) {
		netpaths, err := ParseNetpathPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, netpaths)
	})

	t.Run("ParseNetpathPayload should return empty Netpath array on empty json object", func(t *testing.T) {
		netpaths, err := ParseNetpathPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, netpaths)
	})

	t.Run("ParseNetpathPayload should return a valid Netpath on valid payload", func(t *testing.T) {
		netpaths, err := ParseNetpathPayload(api.Payload{Data: netpathData, Encoding: encodingGzip})
		assert.NoError(t, err)

		assert.Len(t, netpaths, 1)
		np := netpaths[0]

		assert.Equal(t, int64(1737933404281), np.Timestamp)
		assert.Equal(t, "7.64.0-devel+git.40.38beef2", np.AgentVersion)
		assert.Equal(t, "default", np.Namespace)
		assert.Equal(t, "da6f9055-b7df-41b0-bafd-0e5d3c6c370e", np.PathtraceID)
		assert.Equal(t, payload.PathOrigin("network_path_integration"), np.Origin)
		assert.Equal(t, payload.Protocol("TCP"), np.Protocol)
		assert.Equal(t, "i-019fda1a9f830d95e", np.Source.Hostname)
		assert.Equal(t, "subnet-091570395d476e9ce", np.Source.Via.Subnet.Alias)
		assert.Equal(t, "vpc-029c0faf8f49dee8d", np.Source.NetworkID)
		assert.Equal(t, "api.datadoghq.eu", np.Destination.Hostname)
		assert.Equal(t, "34.107.236.155", np.Destination.IPAddress)
		assert.Equal(t, uint16(443), np.Destination.Port)
		assert.Equal(t, "155.236.107.34.bc.googleusercontent.com", np.Destination.ReverseDNSHostname)

		assert.Len(t, np.Hops, 9)
		assert.Equal(t, payload.NetworkPathHop{
			TTL:       1,
			IPAddress: "10.1.62.52",
			Hostname:  "ip-10-1-62-52.ec2.internal",
			RTT:       0.39,
			Reachable: true,
		}, np.Hops[0])
		assert.Equal(t, payload.NetworkPathHop{
			TTL:       2,
			IPAddress: "unknown_hop_2",
			Hostname:  "unknown_hop_2",
			Reachable: false,
		}, np.Hops[1])
		assert.Equal(t, payload.NetworkPathHop{
			TTL:       9,
			IPAddress: "34.107.236.155",
			Hostname:  "155.236.107.34.bc.googleusercontent.com",
			RTT:       2.864,
			Reachable: true,
		}, np.Hops[8])
	})
}
