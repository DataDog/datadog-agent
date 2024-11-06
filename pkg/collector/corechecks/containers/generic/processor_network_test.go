// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestNetworkProcessorExtension(t *testing.T) {
	mockSender := mocksender.NewMockSender("network-extension")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	mockCollector := mock.NewCollector("testCollector")

	networkProcessor := NewProcessorNetwork()

	// Test setup:
	// container1 & container2 share the same network namespace with a POD owner (should report metrics once with orchestrator tags)
	// container3 has no namespace information (should report with high tags)
	// container4 is standalone is a namespace (should report with high tags)
	// container5 is using host network (should not report at all)
	// container6 & container7 share the same network namespace with unknown owner  (should not report at all)
	podEntityID := types.NewEntityID(types.KubernetesPodUID, "pod1")
	fakeTagger.SetTags(podEntityID, "foo", []string{"low:common"}, []string{"orch:common12", "pod:test"}, nil, nil)
	container1 := CreateContainerMeta("docker", "1")
	container1.Owner = &workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "pod1",
	}
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container1.ID), "foo", []string{"low:common"}, []string{"orch:common12"}, []string{"id:container1"}, nil)
	mockCollector.SetContainerEntry(container1.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(12.0),
			BytesRcvd:   pointer.Ptr(12.0),
			PacketsSent: pointer.Ptr(12.0),
			PacketsRcvd: pointer.Ptr(12.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(12.0),
					BytesRcvd:   pointer.Ptr(12.0),
					PacketsSent: pointer.Ptr(12.0),
					PacketsRcvd: pointer.Ptr(12.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(100)),
			UsingHostNetwork:        pointer.Ptr(false),
		},
	})

	container2 := CreateContainerMeta("docker", "2")
	container1.Owner = &workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "pod1",
	}
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container2.ID), "foo", []string{"low:common"}, []string{"orch:common12"}, []string{"id:container2"}, nil)
	mockCollector.SetContainerEntry(container2.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(12.0),
			BytesRcvd:   pointer.Ptr(12.0),
			PacketsSent: pointer.Ptr(12.0),
			PacketsRcvd: pointer.Ptr(12.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(12.0),
					BytesRcvd:   pointer.Ptr(12.0),
					PacketsSent: pointer.Ptr(12.0),
					PacketsRcvd: pointer.Ptr(12.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(100)),
			UsingHostNetwork:        pointer.Ptr(false),
		},
	})

	container3 := CreateContainerMeta("docker", "3")
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container3.ID), "foo", []string{"low:common"}, []string{"orch:standalone3"}, []string{"id:container3"}, nil)
	mockCollector.SetContainerEntry(container3.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(3.0),
			BytesRcvd:   pointer.Ptr(3.0),
			PacketsSent: pointer.Ptr(3.0),
			PacketsRcvd: pointer.Ptr(3.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(3.0),
					BytesRcvd:   pointer.Ptr(3.0),
					PacketsSent: pointer.Ptr(3.0),
					PacketsRcvd: pointer.Ptr(3.0),
				},
			},
		},
	})

	container4 := CreateContainerMeta("docker", "4")
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container4.ID), "foo", []string{"low:common"}, []string{"orch:standalone4"}, []string{"id:container4"}, nil)
	mockCollector.SetContainerEntry(container4.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(4.0),
			BytesRcvd:   pointer.Ptr(4.0),
			PacketsSent: pointer.Ptr(4.0),
			PacketsRcvd: pointer.Ptr(4.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(4.0),
					BytesRcvd:   pointer.Ptr(4.0),
					PacketsSent: pointer.Ptr(4.0),
					PacketsRcvd: pointer.Ptr(4.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(400)),
			UsingHostNetwork:        pointer.Ptr(false),
		},
	})

	container5 := CreateContainerMeta("docker", "5")
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container5.ID), "foo", []string{"low:common"}, []string{"orch:standalone5"}, []string{"id:container5"}, nil)
	mockCollector.SetContainerEntry(container5.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(5.0),
			BytesRcvd:   pointer.Ptr(5.0),
			PacketsSent: pointer.Ptr(5.0),
			PacketsRcvd: pointer.Ptr(5.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(5.0),
					BytesRcvd:   pointer.Ptr(5.0),
					PacketsSent: pointer.Ptr(5.0),
					PacketsRcvd: pointer.Ptr(5.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(1)),
			UsingHostNetwork:        pointer.Ptr(true),
		},
	})

	container6 := CreateContainerMeta("docker", "6")
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container6.ID), "foo", []string{"low:common"}, []string{"orch:common12"}, []string{"id:container6"}, nil)
	mockCollector.SetContainerEntry(container6.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(12.0),
			BytesRcvd:   pointer.Ptr(12.0),
			PacketsSent: pointer.Ptr(12.0),
			PacketsRcvd: pointer.Ptr(12.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(12.0),
					BytesRcvd:   pointer.Ptr(12.0),
					PacketsSent: pointer.Ptr(12.0),
					PacketsRcvd: pointer.Ptr(12.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(101)),
			UsingHostNetwork:        pointer.Ptr(false),
		},
	})

	container7 := CreateContainerMeta("docker", "7")
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, container2.ID), "foo", []string{"low:common"}, []string{"orch:common12"}, []string{"id:container7"}, nil)
	mockCollector.SetContainerEntry(container7.ID, mock.ContainerEntry{
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Ptr(12.0),
			BytesRcvd:   pointer.Ptr(12.0),
			PacketsSent: pointer.Ptr(12.0),
			PacketsRcvd: pointer.Ptr(12.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(12.0),
					BytesRcvd:   pointer.Ptr(12.0),
					PacketsSent: pointer.Ptr(12.0),
					PacketsRcvd: pointer.Ptr(12.0),
				},
			},
			NetworkIsolationGroupID: pointer.Ptr(uint64(101)),
			UsingHostNetwork:        pointer.Ptr(false),
		},
	})

	// Running them through the ProcessorExtension
	networkProcessor.PreProcess(MockSendMetric, mockSender)

	container1Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "1"), types.HighCardinality)
	networkProcessor.Process(container1Tags, container1, mockCollector, 0)
	container2Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "2"), types.HighCardinality)
	networkProcessor.Process(container2Tags, container2, mockCollector, 0)
	container3Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "3"), types.HighCardinality)
	networkProcessor.Process(container3Tags, container3, mockCollector, 0)
	container4Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "4"), types.HighCardinality)
	networkProcessor.Process(container4Tags, container4, mockCollector, 0)
	container5Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "5"), types.HighCardinality)
	networkProcessor.Process(container5Tags, container5, mockCollector, 0)
	container6Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "6"), types.HighCardinality)
	networkProcessor.Process(container6Tags, container6, mockCollector, 0)
	container7Tags, _ := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "7"), types.HighCardinality)
	networkProcessor.Process(container7Tags, container7, mockCollector, 0)

	networkProcessor.PostProcess(fakeTagger)

	// Checking results
	mockSender.AssertNumberOfCalls(t, "Rate", 12)

	// Container 1 & 2
	mockSender.AssertMetric(t, "Rate", "container.net.sent", 12, "", []string{"low:common", "orch:common12", "pod:test", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.sent.packets", 12, "", []string{"low:common", "orch:common12", "pod:test", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd", 12, "", []string{"low:common", "orch:common12", "pod:test", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd.packets", 12, "", []string{"low:common", "orch:common12", "pod:test", "interface:eth0"})

	// Container 3
	mockSender.AssertMetric(t, "Rate", "container.net.sent", 3, "", []string{"low:common", "orch:standalone3", "id:container3", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.sent.packets", 3, "", []string{"low:common", "orch:standalone3", "id:container3", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd", 3, "", []string{"low:common", "orch:standalone3", "id:container3", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd.packets", 3, "", []string{"low:common", "orch:standalone3", "id:container3", "interface:eth0"})

	// Container 4
	mockSender.AssertMetric(t, "Rate", "container.net.sent", 4, "", []string{"low:common", "orch:standalone4", "id:container4", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.sent.packets", 4, "", []string{"low:common", "orch:standalone4", "id:container4", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd", 4, "", []string{"low:common", "orch:standalone4", "id:container4", "interface:eth0"})
	mockSender.AssertMetric(t, "Rate", "container.net.rcvd.packets", 4, "", []string{"low:common", "orch:standalone4", "id:container4", "interface:eth0"})
}
