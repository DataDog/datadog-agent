// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

type mockContainerLister struct {
	containers []workloadmeta.Container
	err        error
}

func (l *mockContainerLister) List() ([]workloadmeta.Container, error) {
	return l.containers, l.err
}

func createTestProcessor(listerContainers []workloadmeta.Container, listerError error, metricsContainers map[string]metrics.MockContainerEntry) (*mocksender.MockSender, *Processor) {
	mockProvider := metrics.NewMockMetricsProvider()
	mockCollector := metrics.NewMockCollector("testCollector")
	mockProvider.RegisterCollector("docker", mockCollector)
	mockProvider.RegisterCollector("containerd", mockCollector)
	for cID, entry := range metricsContainers {
		mockCollector.SetContainerEntry(cID, entry)
	}

	mockLister := mockContainerLister{
		containers: listerContainers,
		err:        listerError,
	}

	filter, _ := containers.GetSharedMetricFilter()

	mockedSender := mocksender.NewMockSender("generic-container")
	mockedSender.SetupAcceptAll()

	p := &Processor{
		metricsProvider: mockProvider,
		ctrLister:       &mockLister,
		metricsAdapter:  GenericMetricsAdapter{},
		ctrFilter:       filter,
	}

	return mockedSender, p
}

func createContainerMeta(runtime, cID string) workloadmeta.Container {
	return workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   cID,
		},
		Runtime: workloadmeta.ContainerRuntime(runtime),
		State: workloadmeta.ContainerState{
			Running:   true,
			StartedAt: time.Now(),
		},
	}
}

func TestProcessorRunFullStatsLinux(t *testing.T) {
	containersMeta := []workloadmeta.Container{
		// Container with full stats
		createContainerMeta("docker", "cID100"),
	}

	containersStats := map[string]metrics.MockContainerEntry{
		"cID100": {
			Error: nil,
			NetworkStats: metrics.ContainerNetworkStats{
				BytesSent:   util.Float64Ptr(42),
				BytesRcvd:   util.Float64Ptr(43),
				PacketsSent: util.Float64Ptr(420),
				PacketsRcvd: util.Float64Ptr(421),
				Interfaces: map[string]metrics.InterfaceNetStats{
					"eth42": {
						BytesSent:   util.Float64Ptr(42),
						BytesRcvd:   util.Float64Ptr(43),
						PacketsSent: util.Float64Ptr(420),
						PacketsRcvd: util.Float64Ptr(421),
					},
				},
			},
			ContainerStats: metrics.ContainerStats{
				CPU: &metrics.ContainerCPUStats{
					Total:            util.Float64Ptr(100),
					System:           util.Float64Ptr(200),
					User:             util.Float64Ptr(300),
					Shares:           util.Float64Ptr(400),
					Limit:            util.Float64Ptr(50),
					ElapsedPeriods:   util.Float64Ptr(500),
					ThrottledPeriods: util.Float64Ptr(0),
					ThrottledTime:    util.Float64Ptr(100),
				},
				Memory: &metrics.ContainerMemStats{
					UsageTotal:   util.Float64Ptr(100),
					KernelMemory: util.Float64Ptr(40),
					Limit:        util.Float64Ptr(42000),
					Softlimit:    util.Float64Ptr(40000),
					RSS:          util.Float64Ptr(300),
					Cache:        util.Float64Ptr(200),
					Swap:         util.Float64Ptr(0),
					OOMEvents:    util.Float64Ptr(10),
				},
				IO: &metrics.ContainerIOStats{
					Devices: map[string]metrics.DeviceIOStats{
						"/dev/foo": {
							ReadBytes:       util.Float64Ptr(100),
							WriteBytes:      util.Float64Ptr(200),
							ReadOperations:  util.Float64Ptr(10),
							WriteOperations: util.Float64Ptr(20),
						},
						"/dev/bar": {
							ReadBytes:       util.Float64Ptr(100),
							WriteBytes:      util.Float64Ptr(200),
							ReadOperations:  util.Float64Ptr(10),
							WriteOperations: util.Float64Ptr(20),
						},
					},
					ReadBytes:       util.Float64Ptr(200),
					WriteBytes:      util.Float64Ptr(400),
					ReadOperations:  util.Float64Ptr(20),
					WriteOperations: util.Float64Ptr(40),
				},
				PID: &metrics.ContainerPIDStats{
					PIDs:        []int{4, 2},
					ThreadCount: util.Float64Ptr(10),
					ThreadLimit: util.Float64Ptr(20),
				},
			},
		},
	}

	mockSender, processor := createTestProcessor(containersMeta, nil, containersStats)
	err := processor.Run(mockSender, 0)
	assert.ErrorIs(t, err, nil)

	expectedTags := []string{"runtime:docker"}
	mockSender.AssertNumberOfCalls(t, "Rate", 13)
	mockSender.AssertNumberOfCalls(t, "Gauge", 13)

	mockSender.AssertMetricInRange(t, "Gauge", "container.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "container.cpu.usage", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "container.cpu.user", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "container.cpu.system", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "container.cpu.throttled.time", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "container.cpu.throttled.periods", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.cpu.shares", 400, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.cpu.limit", 500000000, "", expectedTags)

	mockSender.AssertMetric(t, "Gauge", "container.memory.usage", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.kernel", 40, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.limit", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.soft_limit", 40000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.rss", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.cache", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.swap", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.memory.oomevents", 10, "", expectedTags)

	expectedFooTags := extraTags(expectedTags, "device_name:/dev/foo")
	mockSender.AssertMetric(t, "Rate", "container.io.read", 100, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "container.io.read.operations", 10, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "container.io.write", 200, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "container.io.write.operations", 20, "", expectedFooTags)
	expectedBarTags := extraTags(expectedTags, "device_name:/dev/bar")
	mockSender.AssertMetric(t, "Rate", "container.io.read", 100, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "container.io.read.operations", 10, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "container.io.write", 200, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "container.io.write.operations", 20, "", expectedBarTags)

	mockSender.AssertMetric(t, "Gauge", "container.pid.thread_count", 10, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "container.pid.thread_limit", 20, "", expectedTags)
}

func TestProcessorRunPartialStats(t *testing.T) {
	containersMeta := []workloadmeta.Container{
		// Container without stats
		createContainerMeta("containerd", "cID201"),
		// Container with explicit error
		createContainerMeta("containerd", "cID202"),
	}

	containersStats := map[string]metrics.MockContainerEntry{
		"cID202": {
			Error: fmt.Errorf("Unable to read some stuff"),
		},
	}

	mockSender, processor := createTestProcessor(containersMeta, nil, containersStats)
	err := processor.Run(mockSender, 0)
	assert.ErrorIs(t, err, nil)

	mockSender.AssertNumberOfCalls(t, "Rate", 0)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
}
