// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build cri

package containers

import (
	"testing"
	"time"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri/crimock"
	"github.com/stretchr/testify/mock"
)

func TestCriGenerateMetrics(t *testing.T) {
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

	mockedCriUtil := new(crimock.MockCRIClient)
	mockedCriUtil.On("GetContainerStatus", "cri://foobar").Return(&pb.ContainerStatus{
		StartedAt: time.Now().UnixNano() - int64(42*time.Second),
	}, nil)
	mockedSender := mocksender.NewMockSender(criCheck.ID())
	mockedSender.On("Gauge", "cri.mem.rss", float64(0), "", []string{"runtime:fakeruntime"})
	mockedSender.On("Rate", "cri.cpu.usage", float64(0), "", []string{"runtime:fakeruntime"})
	mockedSender.On("Gauge", "cri.disk.used", float64(0), "", []string{"runtime:fakeruntime"})
	mockedSender.On("Gauge", "cri.disk.inodes", float64(0), "", []string{"runtime:fakeruntime"})
	mockedSender.On("Gauge", "cri.uptime", mock.MatchedBy(func(uptime float64) bool { return uptime >= 42.0 }), "", []string{"runtime:fakeruntime"})
	criCheck.generateMetrics(mockedSender, stats, mockedCriUtil)
}
