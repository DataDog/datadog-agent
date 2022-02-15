// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && (linux || windows)
// +build docker
// +build linux windows

package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
)

func Test_convertNetworkStats(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]types.NetworkStats
		networks       map[string]string
		expectedOutput provider.ContainerNetworkStats
	}{
		{
			name: "basic",
			input: map[string]types.NetworkStats{
				"eth0": {
					RxBytes:   42,
					RxPackets: 43,
					TxBytes:   44,
					TxPackets: 45,
				},
				"eth1": {
					RxBytes:   46,
					RxPackets: 47,
					TxBytes:   48,
					TxPackets: 49,
				},
			},
			expectedOutput: provider.ContainerNetworkStats{
				BytesSent:   util.Float64Ptr(92),
				BytesRcvd:   util.Float64Ptr(88),
				PacketsSent: util.Float64Ptr(94),
				PacketsRcvd: util.Float64Ptr(90),
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {
						BytesSent:   util.Float64Ptr(44),
						BytesRcvd:   util.Float64Ptr(42),
						PacketsSent: util.Float64Ptr(45),
						PacketsRcvd: util.Float64Ptr(43),
					},
					"eth1": {
						BytesSent:   util.Float64Ptr(48),
						BytesRcvd:   util.Float64Ptr(46),
						PacketsSent: util.Float64Ptr(49),
						PacketsRcvd: util.Float64Ptr(47),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertNetworkStats(test.input))
		})
	}
}
