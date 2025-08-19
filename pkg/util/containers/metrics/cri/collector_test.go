// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/DataDog/datadog-agent/pkg/util/containers/cri/crimock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestGetContainerStats(t *testing.T) {
	containerID := "123"

	mockedCriClient := new(crimock.MockCRIClient)
	mockedCriClient.On("GetContainerStats", containerID).Return(
		&pb.ContainerStats{
			Attributes: &pb.ContainerAttributes{
				Id: containerID,
			},
			Cpu: &pb.CpuUsage{
				UsageCoreNanoSeconds: &pb.UInt64Value{
					Value: 1000,
				},
			},
			Memory: &pb.MemoryUsage{
				WorkingSetBytes: &pb.UInt64Value{
					Value: 1024,
				},
				UsageBytes: &pb.UInt64Value{
					Value: 2048,
				},
				RssBytes: &pb.UInt64Value{
					Value: 512,
				},
			},
		},
		nil,
	)

	collector := criCollector{
		client: mockedCriClient,
	}

	stats, err := collector.GetContainerStats("", containerID, 10*time.Second)
	assert.NoError(t, err)

	assert.Equal(t, pointer.Ptr(1000.0), stats.CPU.Total)
	assert.Equal(t, pointer.Ptr(1024.0), stats.Memory.WorkingSet)
	assert.Equal(t, pointer.Ptr(2048.0), stats.Memory.UsageTotal)
	assert.Equal(t, pointer.Ptr(512.0), stats.Memory.RSS)
}
