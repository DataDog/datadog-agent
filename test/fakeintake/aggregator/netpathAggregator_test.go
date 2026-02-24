// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"net"
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

		assert.Equal(t, int64(1757494901878), np.Timestamp)
		assert.Equal(t, "7.72.0-devel+git.93.cb80d71", np.AgentVersion)
		assert.Equal(t, "default", np.Namespace)
		assert.Equal(t, "bf6a0ba9-8823-4089-a3ff-ff9f7b7ff978", np.TestRunID)
		assert.Equal(t, payload.PathOrigin("network_path_integration"), np.Origin)
		assert.Equal(t, payload.Protocol("TCP"), np.Protocol)
		assert.Equal(t, "my-host", np.Source.Hostname)
		assert.Equal(t, "subnet-091570395d476e9ce", np.Source.Via.Subnet.Alias)
		assert.Equal(t, "vpc-029c0faf8f49dee8d", np.Source.NetworkID)
		assert.Equal(t, "google.com", np.Destination.Hostname)
		assert.Equal(t, uint16(443), np.Destination.Port)

		assert.Len(t, np.Traceroute.Runs, 3)
		run1 := np.Traceroute.Runs[0]
		assert.Equal(t, payload.TracerouteHop{
			TTL:       1,
			IPAddress: net.ParseIP("192.168.1.1"),
			RTT:       4.494625,
			Reachable: true,
		}, run1.Hops[0])
		assert.Equal(t, payload.TracerouteHop{
			TTL:       2,
			Reachable: false,
		}, run1.Hops[1])
		assert.Equal(t, payload.TracerouteHop{
			TTL:        3,
			IPAddress:  net.ParseIP("80.10.45.2"),
			ReverseDNS: []string{"xxx.rbci.orange.net."},
			RTT:        6.245875,
			Reachable:  true,
		}, run1.Hops[2])
	})
}
