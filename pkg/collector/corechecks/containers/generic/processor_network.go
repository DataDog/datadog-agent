// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggerUtils "github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type containerNetwork struct {
	stats *metrics.ContainerNetworkStats
	tags  []string
}

type groupedContainerNetwork struct {
	count uint
	owner *workloadmeta.EntityID
	stats *metrics.ContainerNetworkStats
	tags  []string
}

// ProcessorNetwork is a Processor extension taking care of network metrics
type ProcessorNetwork struct {
	sender    SenderFunc
	aggSender sender.Sender

	groupedContainerNetwork   map[uint64]*groupedContainerNetwork
	ungroupedContainerNetwork []containerNetwork
}

// NewProcessorNetwork returns a default ProcessorExtension
func NewProcessorNetwork() ProcessorExtension {
	return &ProcessorNetwork{}
}

// PreProcess is called once during check run, before any call to `Process`
func (pn *ProcessorNetwork) PreProcess(sender SenderFunc, aggSender sender.Sender) {
	pn.sender = sender
	pn.aggSender = aggSender
	pn.groupedContainerNetwork = make(map[uint64]*groupedContainerNetwork)
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

		if containerNetworkStats.NetworkIsolationGroupID != nil {
			groupID := *containerNetworkStats.NetworkIsolationGroupID
			if groupedNetwork := pn.groupedContainerNetwork[groupID]; groupedNetwork == nil {
				pn.groupedContainerNetwork[groupID] = &groupedContainerNetwork{count: 1, stats: containerNetworkStats, tags: tags, owner: container.Owner}
			} else {
				groupedNetwork.count++
			}
		} else {
			pn.ungroupedContainerNetwork = append(pn.ungroupedContainerNetwork, containerNetwork{stats: containerNetworkStats, tags: tags})
		}
	}
}

// PostProcess actually computes the metrics
func (pn *ProcessorNetwork) PostProcess(tagger tagger.Component) {
	pn.processGroupedContainerNetwork(tagger)
}

func (pn *ProcessorNetwork) processGroupedContainerNetwork(tagger tagger.Component) {
	// Handle all containers that we could not link to any network isolation group
	for _, containerNetwork := range pn.ungroupedContainerNetwork {
		pn.generateNetworkMetrics(containerNetwork.tags, containerNetwork.stats)
	}
	pn.ungroupedContainerNetwork = nil

	for _, containerNetworks := range pn.groupedContainerNetwork {
		// If we have multiple containers, tagging with container tag is incorrect as the metrics refer to whole isolation group.
		// We need tags for the isolation group, which we cannot always know.
		// The only case we can support is Kubernetes, where we can tag with the pod tags.
		// In other cases we cannot generate accurate container network metrics.
		if containerNetworks.count == 1 {
			pn.generateNetworkMetrics(containerNetworks.tags, containerNetworks.stats)
		} else if containerNetworks.owner != nil && containerNetworks.owner.Kind == workloadmeta.KindKubernetesPod {
			podEntityID := types.NewEntityID(types.KubernetesPodUID, containerNetworks.owner.ID)
			orchTags, err := tagger.Tag(podEntityID, types.HighCardinality)
			if err != nil {
				log.Debugf("Unable to get orchestrator tags for pod: %s", containerNetworks.owner.ID)
				continue
			}

			pn.generateNetworkMetrics(orchTags, containerNetworks.stats)
		}
		// else: Should we merge all tags together? It might be misleading when filtering by container name without grouping by duplicated tags.
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
