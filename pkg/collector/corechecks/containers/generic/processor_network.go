// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type containerNetwork struct {
	stats       *metrics.ContainerNetworkStats
	containerID string
	tags        []string
}

// ProcessorNetwork is a Processor extension taking care of network metrics
type ProcessorNetwork struct {
	sender                  SenderFunc
	aggSender               sender.Sender
	groupedContainerNetwork map[uint64][]containerNetwork
}

// NewProcessorNetwork returns a default ProcessorExtension
func NewProcessorNetwork() ProcessorExtension {
	return &ProcessorNetwork{}
}

// PreProcess is called once during check run, before any call to `Process`
func (pn *ProcessorNetwork) PreProcess(sender SenderFunc, aggSender sender.Sender) {
	pn.sender = sender
	pn.aggSender = aggSender
	pn.groupedContainerNetwork = make(map[uint64][]containerNetwork)
}

// Process stores each container in relevant network group
func (pn *ProcessorNetwork) Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration) {
	containerNetworkStats, err := collector.GetContainerNetworkStats(container.Namespace, container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering network metrics for container: %v failed, metrics may be missing, err: %v", container, err)
		return
	}

	if containerNetworkStats != nil {
		// Do not report metrics for containers running in Host network mode
		if containerNetworkStats.UsingHostNetwork != nil && *containerNetworkStats.UsingHostNetwork {
			return
		}

		var groupID uint64
		if containerNetworkStats.NetworkIsolationGroupID != nil {
			groupID = *containerNetworkStats.NetworkIsolationGroupID
		} else {
			groupID = 0 // Not grouped
		}

		pn.groupedContainerNetwork[groupID] = append(pn.groupedContainerNetwork[groupID], containerNetwork{stats: containerNetworkStats, containerID: container.ID, tags: tags})
	}
}

// PostProcess actually computes the metrics
func (pn *ProcessorNetwork) PostProcess() {
	pn.processGroupedContainerNetwork()
}

func (pn *ProcessorNetwork) processGroupedContainerNetwork() {
	// Handle all containers that we could not link to any network isolation group
	if noGroupContainers, found := pn.groupedContainerNetwork[0]; found {
		for _, containerNetwork := range noGroupContainers {
			pn.generateNetworkMetrics(containerNetwork.tags, containerNetwork.stats)
		}
	}
	delete(pn.groupedContainerNetwork, 0)

	for _, containerNetworks := range pn.groupedContainerNetwork {
		if len(containerNetworks) == 1 {
			pn.generateNetworkMetrics(containerNetworks[0].tags, containerNetworks[0].stats)
		} else {
			// If we have multiple containers, we cannot tag with HighCardinality, so re-tagging with Orchestrator card.
			orchTags, err := tagger.Tag(containers.BuildTaggerEntityName(containerNetworks[0].containerID), collectors.OrchestratorCardinality)
			if err != nil {
				log.Debugf("Unable to get orchestrator tags for container: %s", containerNetworks[0].containerID)
				continue
			}

			pn.generateNetworkMetrics(orchTags, containerNetworks[0].stats)
		}
	}
}

func (pn *ProcessorNetwork) generateNetworkMetrics(tags []string, networkStats *metrics.ContainerNetworkStats) {
	for interfaceName, interfaceStats := range networkStats.Interfaces {
		interfaceTags := taggerUtils.ConcatenateStringTags(tags, "interface:"+interfaceName)
		pn.sender(pn.aggSender.Rate, "container.net.sent", interfaceStats.BytesSent, interfaceTags)
		pn.sender(pn.aggSender.Rate, "container.net.sent.packets", interfaceStats.PacketsSent, interfaceTags)
		pn.sender(pn.aggSender.Rate, "container.net.rcvd", interfaceStats.BytesRcvd, interfaceTags)
		pn.sender(pn.aggSender.Rate, "container.net.rcvd.packets", interfaceStats.PacketsRcvd, interfaceTags)
	}

	if len(networkStats.Interfaces) == 0 {
		pn.sender(pn.aggSender.Rate, "container.net.sent", networkStats.BytesSent, tags)
		pn.sender(pn.aggSender.Rate, "container.net.sent.packets", networkStats.PacketsSent, tags)
		pn.sender(pn.aggSender.Rate, "container.net.rcvd", networkStats.BytesRcvd, tags)
		pn.sender(pn.aggSender.Rate, "container.net.rcvd.packets", networkStats.PacketsRcvd, tags)
	}
}
