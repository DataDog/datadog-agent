// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	proccontainersmocks "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

// Constant modeled after cacheValidityNoRT to allow for mocking containers functions
const (
	cacheValidityNoRTTest = 2 * time.Second
)

func containerCheckWithMockContainerProvider(t *testing.T) (*ContainerCheck, *proccontainersmocks.MockContainerProvider) {
	mockCtrl := gomock.NewController(t)
	containerProvider := proccontainersmocks.NewMockContainerProvider(mockCtrl)
	sysInfo := &model.SystemInfo{
		Cpus: []*model.CPUInfo{
			{CoreId: "1"},
			{CoreId: "2"},
			{CoreId: "3"},
			{CoreId: "4"},
		},
	}
	hostInfo := &HostInfo{
		SystemInfo: sysInfo,
	}

	return &ContainerCheck{
		hostInfo:          hostInfo,
		containerProvider: containerProvider,
	}, containerProvider
}

func TestContainerCheckPayloads(t *testing.T) {
	check, mockContainerProvider := containerCheckWithMockContainerProvider(t)
	check.maxBatchSize = 2

	// mock containers for testing
	containers := []*model.Container{
		{
			Id: "container-1",
			Addresses: []*model.ContainerAddr{
				{Ip: "10.0.2.15", Port: 32769, Protocol: model.ConnectionType_tcp},
				{Ip: "172.17.0.4", Port: 6379, Protocol: model.ConnectionType_tcp},
			},
		},
		{
			Id: "container-2",
			Addresses: []*model.ContainerAddr{
				{Ip: "172.17.0.2", Port: 80, Protocol: model.ConnectionType_tcp},
			},
		},
		{
			Id: "container-3",
			Addresses: []*model.ContainerAddr{
				{Ip: "172.17.0.3", Port: 88, Protocol: model.ConnectionType_tcp},
			},
		},
	}

	mockContainerProvider.EXPECT().GetContainers(cacheValidityNoRTTest, nil).Return(containers, nil, nil, nil)

	// Test check runs without error using default chunk settings
	result, err := check.Run(testGroupID(0), nil)
	assert.NoError(t, err)

	// Test that result has the proper number of chunks, and that those chunks are of the correct type
	for _, elem := range result.Payloads() {
		assert.IsType(t, &model.CollectorContainer{}, elem)
		collectorContainers := elem.(*model.CollectorContainer)
		resultContainers := collectorContainers.GetContainers()
		for _, ctr := range resultContainers {
			assert.Empty(t, ctr.Host)
		}
		if len(resultContainers) > check.maxBatchSize {
			t.Errorf("Expected less than %d messages in chunk, got %d",
				check.maxBatchSize, len(resultContainers))
		}
	}
}

func TestContainerCheckChunking(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		noChunking            bool
		expectedPayloadLength int
	}{
		{
			name:                  "Chunking",
			noChunking:            false,
			expectedPayloadLength: 2,
		},
		{
			name:                  "No chunking",
			noChunking:            true,
			expectedPayloadLength: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			containerCheck, mockContainerProvider := containerCheckWithMockContainerProvider(t)

			// Set small size per chunk to force chunking behavior
			containerCheck.maxBatchSize = 1

			// mock containers for testing
			containers := []*model.Container{
				{
					Id: "container-1",
					Addresses: []*model.ContainerAddr{
						{Ip: "10.0.2.15", Port: 32769, Protocol: model.ConnectionType_tcp},
						{Ip: "172.17.0.4", Port: 6379, Protocol: model.ConnectionType_tcp},
					},
				},
				{
					Id: "container-2",
					Addresses: []*model.ContainerAddr{
						{Ip: "172.17.0.2", Port: 80, Protocol: model.ConnectionType_tcp},
					},
				},
			}
			mockContainerProvider.EXPECT().GetContainers(cacheValidityNoRTTest, nil).Return(containers, nil, nil, nil)

			// Test check runs without error and has correct number of chunks
			actual, err := containerCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			require.NoError(t, err)
			assert.Len(t, actual.Payloads(), tc.expectedPayloadLength)
		})
	}
}
