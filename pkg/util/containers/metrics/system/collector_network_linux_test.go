// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestCollectNetworkStats(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-collect-network-stats")
	assert.Nil(t, err)
	defer dummyProcDir.RemoveAll() // clean up

	for _, tc := range []struct {
		pid        int
		name       string
		dev        string
		stat       provider.ContainerNetworkStats
		summedStat *provider.InterfaceNetStats
	}{
		{
			pid:  1245,
			name: "one-container-interface",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			stat: provider.ContainerNetworkStats{
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {
						BytesRcvd:   pointer.Ptr(1345.0),
						PacketsRcvd: pointer.Ptr(10.0),
						BytesSent:   pointer.Ptr(0.0),
						PacketsSent: pointer.Ptr(0.0),
					},
				},
				BytesRcvd:   pointer.Ptr(1345.0),
				PacketsRcvd: pointer.Ptr(10.0),
				BytesSent:   pointer.Ptr(0.0),
				PacketsSent: pointer.Ptr(0.0),
			},
		},
		// Multiple docker networks
		{
			pid:  5153,
			name: "multiple-networks",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			stat: provider.ContainerNetworkStats{
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {
						BytesRcvd:   pointer.Ptr(648.0),
						PacketsRcvd: pointer.Ptr(8.0),
						BytesSent:   pointer.Ptr(0.0),
						PacketsSent: pointer.Ptr(0.0),
					},
					"eth1": {
						BytesRcvd:   pointer.Ptr(1478.0),
						PacketsRcvd: pointer.Ptr(19.0),
						BytesSent:   pointer.Ptr(182.0),
						PacketsSent: pointer.Ptr(3.0),
					},
				},
				BytesRcvd:   pointer.Ptr(2126.0),
				PacketsRcvd: pointer.Ptr(27.0),
				BytesSent:   pointer.Ptr(182.0),
				PacketsSent: pointer.Ptr(3.0),
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(tc.pid), "net", "dev"), tc.dev)
			assert.NoError(t, err)

			stat, err := collectNetworkStats(dummyProcDir.RootPath, tc.pid)
			stat.Timestamp = tc.stat.Timestamp
			assert.NoError(t, err)
			assert.Equal(t, &tc.stat, stat)
		})
	}
}
