// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package containers

import (
	"testing"

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
	ioSum := &cmetrics.CgroupIOStat{
		ReadBytes:  uint64(38989367),
		WriteBytes: uint64(671846455),
	}
	dockerCheck.reportIOMetrics(ioSum, tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(38989367), "", tags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(671846455), "", tags)

	// Test per-device when available
	ioPerDevice := &cmetrics.CgroupIOStat{
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
	}
	sdaTags := append(tags, "device:sda")
	sdbTags := append(tags, "device:sdb")
	dockerCheck.reportIOMetrics(ioPerDevice, tags, mockSender)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(37858816), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(671846400), "", sdaTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", float64(1130496), "", sdbTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", float64(0), "", sdbTags)
}
