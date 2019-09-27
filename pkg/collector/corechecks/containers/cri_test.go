// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cri

package containers

import (
	"testing"

	pb "github.com/kubernetes/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

func TestCRIprocessContainerStats(t *testing.T) {
	criCheck := &CRICheck{
		CheckBase: core.NewCheckBase(criCheckName),
		instance: &CRIConfig{
			CollectDisk: true,
		},
	}

	stats := make(map[string]*pb.ContainerStats)
	stats["cri://foobar"] = &pb.ContainerStats{
		Cpu: &pb.CpuUsage{
			Timestamp: 123456789,
		},
	}

	mocked := mocksender.NewMockSender(criCheck.ID())
	mocked.On("Gauge", "cri.mem.rss", float64(0), "", []string{"runtime:fakeruntime"})
	mocked.On("Rate", "cri.cpu.usage", float64(0), "", []string{"runtime:fakeruntime"})
	mocked.On("Gauge", "cri.disk.used", float64(0), "", []string{"runtime:fakeruntime"})
	mocked.On("Gauge", "cri.disk.inodes", float64(0), "", []string{"runtime:fakeruntime"})
	criCheck.processContainerStats(mocked, "fakeruntime", stats)
}
