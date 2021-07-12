// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker,!darwin

package docker

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	cmetrics "github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

func TestReportIOMetrics(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := mocksender.NewMockSender(dockerCheck.ID())
	mockSender.SetupAcceptAll()

	tags := []string{"constant:tags", "container_name:dummy"}

	// Test fallback to sums when per-device is not available
	ioSum := &cmetrics.ContainerIOStats{
		ReadBytes:  uint64(38989367),
		WriteBytes: uint64(671846455),
	}
	dockerCheck.reportIOMetrics(ioSum, tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(38989367), "", tags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(671846455), "", tags)

	// Test per-device when available
	ioPerDevice := &cmetrics.ContainerIOStats{
		ReadBytes:  uint64(38989367),
		WriteBytes: uint64(671846455),
		DeviceReadBytes: map[string]uint64{
			"sda": 37858816,
			"sdb": 1130496,
		},
		DeviceWriteBytes: map[string]uint64{
			"sda": 671846400,
			"sdb": 0,
		},
		DeviceReadOperations: map[string]uint64{
			"sda": 1042,
			"sdb": 42,
		},
		DeviceWriteOperations: map[string]uint64{
			"sda": 2042,
			"sdb": 1042,
		},
	}
	sdaTags := append(tags, "device:sda", "device_name:sda")
	sdbTags := append(tags, "device:sdb", "device_name:sdb")
	dockerCheck.reportIOMetrics(ioPerDevice, tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(37858816), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(671846400), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(1130496), "", sdbTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(0), "", sdbTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_operations", float64(1042), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_operations", float64(2042), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_operations", float64(42), "", sdbTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_operations", float64(1042), "", sdbTags)
}

func TestReportUptime(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := mocksender.NewMockSender(dockerCheck.ID())
	mockSender.SetupAcceptAll()

	tags := []string{"constant:tags", "container_name:dummy"}
	currentTime := time.Now().Unix()

	startTime := currentTime - 60
	dockerCheck.reportUptime(startTime, currentTime, tags, mockSender)
	mockSender.AssertMetric(t, "Gauge", "docker.uptime", 60.0, "", tags)
}

func TestReportCPUNoLimit(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := mocksender.NewMockSender(dockerCheck.ID())
	mockSender.SetupAcceptAll()

	tags := []string{"constant:tags", "container_name:dummy"}
	testTime := time.Now()
	startTime := testTime.Add(-10 * time.Second)

	cpu := cmetrics.ContainerCPUStats{
		Timestsamp: testTime,
		System:     10,
		User:       10,
		UsageTotal: 20.0,
	}

	// 100% is 1 CPU, 200% is 2 CPUs, etc.
	// So no limit is # of host CPU
	limits := cmetrics.ContainerLimits{
		CPULimit: 100.0,
	}

	dockerCheck.reportCPUMetrics(&cpu, &limits, startTime.Unix(), tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.limit", 1000, "", tags)
}

func TestReportCPULimit(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := mocksender.NewMockSender(dockerCheck.ID())
	mockSender.SetupAcceptAll()

	tags := []string{"constant:tags", "container_name:dummy"}
	testTime := time.Now()
	startTime := testTime.Add(-10 * time.Second)

	cpu := cmetrics.ContainerCPUStats{
		Timestsamp: testTime,
		System:     10,
		User:       10,
		UsageTotal: 20.0,
	}

	limits := cmetrics.ContainerLimits{
		CPULimit: 50,
	}

	dockerCheck.reportCPUMetrics(&cpu, &limits, startTime.Unix(), tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.limit", 500, "", tags)
}
