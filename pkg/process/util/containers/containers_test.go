// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/process"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
)

func TestGetContainers(t *testing.T) {
	// Metrics provider
	metricsCollector := mock.NewCollector("foo")
	metricsProvider := mock.NewMetricsProvider()
	metricsProvider.RegisterConcreteCollector(provider.NewRuntimeMetadata(string(provider.RuntimeNameContainerd), ""), metricsCollector)
	metricsProvider.RegisterConcreteCollector(provider.NewRuntimeMetadata(string(provider.RuntimeNameGarden), ""), metricsCollector)

	// Workload meta + tagger
	metadataProvider := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	// Finally, container provider
	testTime := time.Now()
	filter, err := containers.GetPauseContainerFilter()
	assert.NoError(t, err)
	containerProvider := NewContainerProvider(metricsProvider, metadataProvider, filter, fakeTagger)

	// Containers:
	// cID1 full stats
	// cID2 not running
	// cID3 missing metrics
	// cID4 missing tags
	// cID5 garden container full stats
	// cID6 garden container missing tags
	// cID7 container from pod with exclude annotation

	// cID1 full stats
	cID1Metrics := mock.GetFullSampleContainerEntry()
	cID1Metrics.ContainerStats.Timestamp = testTime
	cID1Metrics.NetworkStats.Timestamp = testTime
	cID1Metrics.PIDs = []int{1, 2, 3}
	metricsCollector.SetContainerEntry("cID1", cID1Metrics)
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID1",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "container1",
			Namespace: "foo",
		},
		NetworkIPs: map[string]string{
			"net1": "10.0.0.1",
			"net2": "192.168.0.1",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     420,
				Protocol: "tcp",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:   "somesha",
			Name: "myapp/foo",
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			Health:    workloadmeta.ContainerHealthHealthy,
			CreatedAt: testTime.Add(-10 * time.Minute),
			StartedAt: testTime,
		},
		Resources: workloadmeta.ContainerResources{
			CPURequest:    pointer.Ptr(500.0),
			MemoryRequest: pointer.Ptr[uint64](300),
		},
	})
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "cID1"), "fake", []string{"low:common"}, []string{"orch:orch1"}, []string{"id:container1"}, nil)

	// cID2 not running
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID2",
		},
	})

	// cID3 missing metrics, still reported
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID3",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "container3",
			Namespace: "foo",
		},
		NetworkIPs: map[string]string{
			"net1": "10.0.0.3",
			"net2": "192.168.0.3",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     423,
				Protocol: "tcp",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			Health:    workloadmeta.ContainerHealthHealthy,
			CreatedAt: testTime.Add(-10 * time.Minute),
			StartedAt: testTime,
		},
		Image: workloadmeta.ContainerImage{
			RepoDigest: "sha256:378e0fa5bc50e6707ec9eb03c511cc6a2a4741f0c345d88dedb2fb9247b19f94",
		},
	})
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "cID3"), "fake", []string{"low:common"}, []string{"orch:orch1"}, []string{"id:container3"}, nil)

	// cID4 missing tags
	cID4Metrics := mock.GetFullSampleContainerEntry()
	cID4Metrics.ContainerStats.Timestamp = testTime
	cID4Metrics.NetworkStats.Timestamp = testTime
	cID4Metrics.PIDs = []int{4, 5}
	metricsCollector.SetContainerEntry("cID4", cID4Metrics)
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID4",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "container4",
			Namespace: "foo",
		},
		NetworkIPs: map[string]string{
			"net1": "10.0.0.4",
			"net2": "192.168.0.4",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     424,
				Protocol: "tcp",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			Health:    workloadmeta.ContainerHealthHealthy,
			CreatedAt: testTime.Add(-10 * time.Minute),
			StartedAt: testTime,
		},
	})

	// cID5 garden container full stats (replacing Linux WorkingSet with CommitBytes)
	cID5Metrics := mock.GetFullSampleContainerEntry()
	cID5Metrics.ContainerStats.CPU.DefaultedLimit = true
	cID5Metrics.ContainerStats.Timestamp = testTime
	cID5Metrics.NetworkStats.Timestamp = testTime
	cID5Metrics.PIDs = []int{6, 7}
	cID5Metrics.ContainerStats.Memory.WorkingSet = nil
	cID5Metrics.ContainerStats.Memory.CommitBytes = pointer.Ptr(355.0)
	metricsCollector.SetContainerEntry("cID5", cID5Metrics)
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID5",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "container5",
		},
		NetworkIPs: map[string]string{
			"": "10.0.0.5",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     425,
				Protocol: "tcp",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeGarden,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			CreatedAt: testTime,
			StartedAt: testTime,
		},
		CollectorTags: []string{"from:pcf", "id:container5"},
	})

	// cID6 garden container missing tags
	metricsCollector.SetContainerEntry("cID6", mock.GetFullSampleContainerEntry())
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID6",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "container6",
		},
		NetworkIPs: map[string]string{
			"": "10.0.0.6",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     426,
				Protocol: "tcp",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeGarden,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			CreatedAt: testTime,
			StartedAt: testTime,
		},
	})

	// cID7 container from pod with exclude annotation
	cID7Metrics := mock.GetFullSampleContainerEntry()
	cID7Metrics.ContainerStats.Timestamp = testTime
	cID7Metrics.NetworkStats.Timestamp = testTime
	cID7Metrics.PIDs = []int{1, 2, 3}
	metricsCollector.SetContainerEntry("cID7", cID7Metrics)
	metadataProvider.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod7",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "foobar-pod7",
			Namespace: "default",
			Annotations: map[string]string{
				"ad.datadoghq.com/container7.exclude": `true`,
			},
		},
		IP: "127.0.0.1",
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "cID7",
				Name: "container7",
			},
		},
	})
	metadataProvider.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "cID7",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "container7",
			Namespace: "foo",
		},
		NetworkIPs: map[string]string{
			"net1": "10.0.0.1",
			"net2": "192.168.0.1",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     420,
				Protocol: "tcp",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:   "somesha",
			Name: "myapp/foo",
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:   true,
			Status:    workloadmeta.ContainerStatusRunning,
			Health:    workloadmeta.ContainerHealthHealthy,
			CreatedAt: testTime.Add(-10 * time.Minute),
			StartedAt: testTime,
		},
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod7",
		},
	})
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "cID7"), "fake", []string{"low:common"}, []string{"orch:orch7"}, []string{"id:container7"}, nil)

	//
	// Running and checking
	///
	processContainers, lastRates, pidToCid, err := containerProvider.GetContainers(0, nil)
	assert.NoError(t, err)
	assert.Empty(t, compareResults(processContainers, []*process.Container{
		{
			Type:          "containerd",
			Id:            "cID1",
			CpuLimit:      50,
			CpuRequest:    500,
			MemoryLimit:   42000,
			MemoryRequest: 300,
			State:         process.ContainerState_running,
			Health:        process.ContainerHealth_healthy,
			Created:       testTime.Add(-10 * time.Minute).Unix(),
			UserPct:       -1,
			SystemPct:     -1,
			TotalPct:      -1,
			CpuUsageNs:    -1,
			MemUsage:      42000,
			MemRss:        300,
			MemAccounted:  350,
			MemCache:      200,
			Started:       testTime.Unix(),
			Tags: []string{
				"low:common",
				"orch:orch1",
				"id:container1",
			},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.1",
					Port:     420,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.1",
					Port:     420,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
		{
			Type:       "containerd",
			Id:         "cID3",
			State:      process.ContainerState_running,
			Health:     process.ContainerHealth_healthy,
			Created:    testTime.Add(-10 * time.Minute).Unix(),
			Started:    testTime.Unix(),
			RepoDigest: "sha256:378e0fa5bc50e6707ec9eb03c511cc6a2a4741f0c345d88dedb2fb9247b19f94",
			Tags: []string{
				"low:common",
				"orch:orch1",
				"id:container3",
			},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.3",
					Port:     423,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.3",
					Port:     423,
					Protocol: process.ConnectionType_tcp,
				},
			},
		},
		{
			Type:         "containerd",
			Id:           "cID4",
			CpuLimit:     50,
			MemoryLimit:  42000,
			State:        process.ContainerState_running,
			Health:       process.ContainerHealth_healthy,
			Created:      testTime.Add(-10 * time.Minute).Unix(),
			UserPct:      -1,
			SystemPct:    -1,
			TotalPct:     -1,
			CpuUsageNs:   -1,
			MemUsage:     42000,
			MemRss:       300,
			MemCache:     200,
			MemAccounted: 350,
			Started:      testTime.Unix(),
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.4",
					Port:     424,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.4",
					Port:     424,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
		{
			Type:         "garden",
			Id:           "cID5",
			CpuLimit:     0,
			MemoryLimit:  42000,
			State:        process.ContainerState_running,
			Created:      testTime.Unix(),
			UserPct:      -1,
			SystemPct:    -1,
			TotalPct:     -1,
			CpuUsageNs:   -1,
			MemUsage:     42000,
			MemRss:       300,
			MemCache:     200,
			MemAccounted: 355,
			Started:      testTime.Unix(),
			Tags:         []string{"from:pcf", "id:container5"},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "10.0.0.5",
					Port:     425,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
	}))
	assert.Equal(t, map[string]*ContainerRateMetrics{
		"cID1": {
			ContainerStatsTimestamp: testTime,
			NetworkStatsTimestamp:   testTime,
			UserCPU:                 300,
			SystemCPU:               200,
			TotalCPU:                100,
			IOReadBytes:             200,
			IOWriteBytes:            400,
			NetworkRcvdBytes:        43,
			NetworkSentBytes:        42,
			NetworkRcvdPackets:      421,
			NetworkSentPackets:      420,
		},
		"cID4": {
			ContainerStatsTimestamp: testTime,
			NetworkStatsTimestamp:   testTime,
			UserCPU:                 300,
			SystemCPU:               200,
			TotalCPU:                100,
			IOReadBytes:             200,
			IOWriteBytes:            400,
			NetworkRcvdBytes:        43,
			NetworkSentBytes:        42,
			NetworkRcvdPackets:      421,
			NetworkSentPackets:      420,
		},
		"cID5": {
			ContainerStatsTimestamp: testTime,
			NetworkStatsTimestamp:   testTime,
			UserCPU:                 300,
			SystemCPU:               200,
			TotalCPU:                100,
			IOReadBytes:             200,
			IOWriteBytes:            400,
			NetworkRcvdBytes:        43,
			NetworkSentBytes:        42,
			NetworkRcvdPackets:      421,
			NetworkSentPackets:      420,
		},
	}, lastRates)
	assert.Equal(t, map[int]string{
		1: "cID1",
		2: "cID1",
		3: "cID1",
		4: "cID4",
		5: "cID4",
		6: "cID5",
		7: "cID5",
	}, pidToCid)

	//
	// Step 2: Test proper rate computation
	//
	cID1Metrics.ContainerStats.Timestamp = testTime.Add(10 * time.Second)
	cID1Metrics.ContainerStats.CPU.User = pointer.Ptr(6000000000.0)
	cID1Metrics.ContainerStats.CPU.System = pointer.Ptr(4000000000.0)
	cID1Metrics.ContainerStats.CPU.Total = pointer.Ptr(2000000000.0)
	cID1Metrics.ContainerStats.IO.ReadBytes = pointer.Ptr(400.0)
	cID1Metrics.ContainerStats.IO.WriteBytes = pointer.Ptr(800.0)
	cID1Metrics.ContainerStats.Memory.UsageTotal = pointer.Ptr(43000.0)
	cID1Metrics.NetworkStats.Timestamp = testTime.Add(10 * time.Second)
	cID1Metrics.NetworkStats.BytesRcvd = pointer.Ptr(83.0)
	cID1Metrics.NetworkStats.BytesSent = pointer.Ptr(82.0)
	cID1Metrics.NetworkStats.PacketsRcvd = pointer.Ptr(821.0)
	cID1Metrics.NetworkStats.PacketsSent = pointer.Ptr(820.0)
	metricsCollector.SetContainerEntry("cID1", cID1Metrics)

	// Remove one container from previous rates
	delete(lastRates, "cID4")

	// Compute stats, normalize CPU to hostCPU
	processContainers, lastRates, pidToCid, err = containerProvider.GetContainers(0, lastRates)
	assert.NoError(t, err)
	assert.Empty(t, compareResults(processContainers, []*process.Container{
		{
			Type:          "containerd",
			Id:            "cID1",
			CpuLimit:      50,
			CpuRequest:    500,
			MemoryLimit:   42000,
			MemoryRequest: 300,
			State:         process.ContainerState_running,
			Health:        process.ContainerHealth_healthy,
			Created:       testTime.Add(-10 * time.Minute).Unix(),
			UserPct:       60,
			SystemPct:     40,
			TotalPct:      20,
			CpuUsageNs:    199999984,
			MemUsage:      43000,
			MemRss:        300,
			MemCache:      200,
			MemAccounted:  350,
			Rbps:          20,
			Wbps:          40,
			NetRcvdPs:     40,
			NetSentPs:     40,
			NetRcvdBps:    4,
			NetSentBps:    4,
			Started:       testTime.Unix(),
			Tags: []string{
				"low:common",
				"orch:orch1",
				"id:container1",
			},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.1",
					Port:     420,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.1",
					Port:     420,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
		{
			Type:       "containerd",
			Id:         "cID3",
			State:      process.ContainerState_running,
			Health:     process.ContainerHealth_healthy,
			Created:    testTime.Add(-10 * time.Minute).Unix(),
			Started:    testTime.Unix(),
			RepoDigest: "sha256:378e0fa5bc50e6707ec9eb03c511cc6a2a4741f0c345d88dedb2fb9247b19f94",
			Tags: []string{
				"low:common",
				"orch:orch1",
				"id:container3",
			},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.3",
					Port:     423,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.3",
					Port:     423,
					Protocol: process.ConnectionType_tcp,
				},
			},
		},
		{
			Type:         "containerd",
			Id:           "cID4",
			CpuLimit:     50,
			MemoryLimit:  42000,
			State:        process.ContainerState_running,
			Health:       process.ContainerHealth_healthy,
			Created:      testTime.Add(-10 * time.Minute).Unix(),
			UserPct:      -1,
			SystemPct:    -1,
			TotalPct:     -1,
			CpuUsageNs:   -1,
			MemUsage:     42000,
			MemRss:       300,
			MemCache:     200,
			MemAccounted: 350,
			Rbps:         0,
			Wbps:         0,
			NetRcvdPs:    0,
			NetSentPs:    0,
			NetRcvdBps:   0,
			NetSentBps:   0,
			Started:      testTime.Unix(),
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "192.168.0.4",
					Port:     424,
					Protocol: process.ConnectionType_tcp,
				},
				{
					Ip:       "10.0.0.4",
					Port:     424,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
		{
			Type:         "garden",
			Id:           "cID5",
			CpuLimit:     0,
			MemoryLimit:  42000,
			State:        process.ContainerState_running,
			Created:      testTime.Unix(),
			UserPct:      0,
			SystemPct:    0,
			TotalPct:     0,
			CpuUsageNs:   0,
			MemUsage:     42000,
			MemRss:       300,
			MemCache:     200,
			MemAccounted: 355,
			Started:      testTime.Unix(),
			Tags:         []string{"from:pcf", "id:container5"},
			Addresses: []*process.ContainerAddr{
				{
					Ip:       "10.0.0.5",
					Port:     425,
					Protocol: process.ConnectionType_tcp,
				},
			},
			ThreadCount: 10,
			ThreadLimit: 20,
		},
	}))
	assert.Equal(t, map[string]*ContainerRateMetrics{
		"cID1": {
			ContainerStatsTimestamp: testTime.Add(10 * time.Second),
			NetworkStatsTimestamp:   testTime.Add(10 * time.Second),
			UserCPU:                 6000000000,
			SystemCPU:               4000000000,
			TotalCPU:                2000000000,
			IOReadBytes:             400,
			IOWriteBytes:            800,
			NetworkRcvdBytes:        83,
			NetworkSentBytes:        82,
			NetworkRcvdPackets:      821,
			NetworkSentPackets:      820,
		},
		"cID4": {
			ContainerStatsTimestamp: testTime,
			NetworkStatsTimestamp:   testTime,
			UserCPU:                 300,
			SystemCPU:               200,
			TotalCPU:                100,
			IOReadBytes:             200,
			IOWriteBytes:            400,
			NetworkRcvdBytes:        43,
			NetworkSentBytes:        42,
			NetworkRcvdPackets:      421,
			NetworkSentPackets:      420,
		},
		"cID5": {
			ContainerStatsTimestamp: testTime,
			NetworkStatsTimestamp:   testTime,
			UserCPU:                 300,
			SystemCPU:               200,
			TotalCPU:                100,
			IOReadBytes:             200,
			IOWriteBytes:            400,
			NetworkRcvdBytes:        43,
			NetworkSentBytes:        42,
			NetworkRcvdPackets:      421,
			NetworkSentPackets:      420,
		},
	}, lastRates)
	assert.Equal(t, map[int]string{
		1: "cID1",
		2: "cID1",
		3: "cID1",
		4: "cID4",
		5: "cID4",
		6: "cID5",
		7: "cID5",
	}, pidToCid)
}

func compareResults(a, b interface{}) string {
	return cmp.Diff(a, b,
		cmpopts.SortSlices(func(x, y interface{}) bool {
			return fmt.Sprintf("%v", x) < fmt.Sprintf("%v", y)
		}),
		cmp.Comparer(func(x, y float32) bool {
			return math.Abs(float64(x-y)) < 0.1
		}),
	)
}
