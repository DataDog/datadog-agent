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
	t.Helper()
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

func TestContainerCheckWithChunking(t *testing.T) {
	containerCheck, mockContainerProvider := containerCheckWithMockContainerProvider(t)

	// Set small size per chunk to encourage chunking behavior
	containerCheck.maxBatchSize = 1

	ctr1 := &model.Container{
		Id: "container-1",
		Addresses: []*model.ContainerAddr{
			{
				Ip:       "10.0.2.15",
				Port:     32769,
				Protocol: model.ConnectionType_tcp,
			},
			{
				Ip:       "172.17.0.4",
				Port:     6379,
				Protocol: model.ConnectionType_tcp,
			},
		},
	}
	ctr2 := &model.Container{
		Id: "container-2",
		Addresses: []*model.ContainerAddr{
			{
				Ip:       "172.17.0.2",
				Port:     80,
				Protocol: model.ConnectionType_tcp,
			},
		},
	}

	containers := []*model.Container{ctr1, ctr2}
	mockContainerProvider.EXPECT().GetContainers(cacheValidityNoRTTest, nil).Return(containers, nil, nil, nil)

	expected := []model.MessageBody{
		&model.CollectorContainer{
			Containers: []*model.Container{ctr1},
			Info:       containerCheck.hostInfo.SystemInfo,
			GroupSize:  int32(len(containers)),
		},
		&model.CollectorContainer{
			Containers: []*model.Container{ctr2},
			Info:       containerCheck.hostInfo.SystemInfo,
			GroupSize:  int32(len(containers)),
		},
	}

	// Test check runs without error
	actual, err := containerCheck.Run(func() int32 { return 0 }, &RunOptions{RunStandard: true, NoChunking: false})
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads())
}

func TestContainerCheckWithoutChunking(t *testing.T) {
	containerCheck, mockContainerProvider := containerCheckWithMockContainerProvider(t)

	// Set small size per chunk to encourage chunking behavior
	containerCheck.maxBatchSize = 1

	ctr1 := &model.Container{
		Id: "container-1",
		Addresses: []*model.ContainerAddr{
			{
				Ip:       "10.0.2.15",
				Port:     32769,
				Protocol: model.ConnectionType_tcp,
			},
			{
				Ip:       "172.17.0.4",
				Port:     6379,
				Protocol: model.ConnectionType_tcp,
			},
		},
	}
	ctr2 := &model.Container{
		Id: "container-2",
		Addresses: []*model.ContainerAddr{
			{
				Ip:       "172.17.0.2",
				Port:     80,
				Protocol: model.ConnectionType_tcp,
			},
		},
	}

	containers := []*model.Container{ctr1, ctr2}
	mockContainerProvider.EXPECT().GetContainers(cacheValidityNoRTTest, nil).Return(containers, nil, nil, nil)

	// Test check runs without error
	actual, err := containerCheck.Run(func() int32 { return 0 }, &RunOptions{RunStandard: true, NoChunking: true})
	require.NoError(t, err)

	// Assert to check there is only one chunk and that the nested values of this chunk match expected
	assert.Len(t, actual.Payloads(), 1)
	actualPayloads := actual.Payloads()
	assert.IsType(t, &model.CollectorContainer{}, actualPayloads[0])
	collectorContainer := actualPayloads[0].(*model.CollectorContainer)
	ProcessDiscoveries := collectorContainer.GetContainers()
	assert.ElementsMatch(t, containers, ProcessDiscoveries)
	assert.EqualValues(t, 1, collectorContainer.GetGroupSize())
}
