// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && (linux || windows)

package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestConvertNetworkStats(t *testing.T) {
	tests := []struct {
		name           string
		input          *types.StatsJSON
		networks       map[string]string
		expectedOutput provider.ContainerNetworkStats
	}{
		{
			name: "basic",
			input: &types.StatsJSON{
				Networks: map[string]types.NetworkStats{
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
			},
			expectedOutput: provider.ContainerNetworkStats{
				BytesSent:   pointer.Ptr(92.0),
				BytesRcvd:   pointer.Ptr(88.0),
				PacketsSent: pointer.Ptr(94.0),
				PacketsRcvd: pointer.Ptr(90.0),
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {
						BytesSent:   pointer.Ptr(44.0),
						BytesRcvd:   pointer.Ptr(42.0),
						PacketsSent: pointer.Ptr(45.0),
						PacketsRcvd: pointer.Ptr(43.0),
					},
					"eth1": {
						BytesSent:   pointer.Ptr(48.0),
						BytesRcvd:   pointer.Ptr(46.0),
						PacketsSent: pointer.Ptr(49.0),
						PacketsRcvd: pointer.Ptr(47.0),
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

func TestGetContainerIDForPID(t *testing.T) {
	mockStore := workloadmeta.NewMockStore()
	collector := dockerCollector{
		pidCache:      provider.NewCache(pidCacheGCInterval),
		metadataStore: mockStore,
	}

	mockStore.SetEntity(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID1",
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		PID:     100,
	})

	// Cache is empty, will trigger a full refresh
	cID1, err := collector.GetContainerIDForPID(100, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cID1)

	// Add an entry for PID 200, should not be picked up because full refresh is recent enough
	mockStore.SetEntity(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID2",
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		PID:     200,
	})

	cID2, err := collector.GetContainerIDForPID(200, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "", cID2)

	cID2, err = collector.GetContainerIDForPID(200, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID2", cID2)
}

func Test_fillStatsFromSpec(t *testing.T) {
	tests := []struct {
		name          string
		spec          *types.ContainerJSON
		expectedStats *provider.ContainerStats
	}{
		{
			name: "Empty HostConfig",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{},
				},
			},
			expectedStats: &provider.ContainerStats{
				Memory: &provider.ContainerMemStats{},
			},
		},
		{
			name: "Memory Limit set",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							Memory: 500,
						},
					},
				},
			},
			expectedStats: &provider.ContainerStats{
				Memory: &provider.ContainerMemStats{
					Limit: pointer.Ptr(500.0),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containerStats := &provider.ContainerStats{Memory: &provider.ContainerMemStats{}}
			fillStatsFromSpec(containerStats, tt.spec)
			assert.Equal(t, "", cmp.Diff(*tt.expectedStats, *containerStats))
		})
	}
}
