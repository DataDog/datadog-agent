// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type containerdCustomMetricsExtension struct {
	sender    generic.SenderFunc
	aggSender aggregator.Sender
}

func (cext *containerdCustomMetricsExtension) PreProcess(sender generic.SenderFunc, aggSender aggregator.Sender) {
	cext.sender = sender
	cext.aggSender = aggSender
}

func (cext *containerdCustomMetricsExtension) Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration) {
	// Duplicate call with generic.Processor, but cache should allow for a fast response.
	containerStats, err := collector.GetContainerStats(container.Namespace, container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering container metrics for container: %v failed, metrics may be missing, err: %v", container, err)
		return
	}

	if containerStats == nil {
		log.Debugf("Metrics provider returned nil stats for container: %v", container)
		return
	}

	if containerStats.IO != nil {
		for deviceName, deviceStats := range containerStats.IO.Devices {
			readDeviceTags := taggerUtils.ConcatenateStringTags(tags, "device:"+deviceName, "device_name:"+deviceName, "operation:read")
			cext.sender(cext.aggSender.Rate, "containerd.blkio.service_recursive_bytes", deviceStats.ReadBytes, readDeviceTags)
			cext.sender(cext.aggSender.Rate, "containerd.blkio.serviced_recursive", deviceStats.ReadOperations, readDeviceTags)

			writeDeviceTags := taggerUtils.ConcatenateStringTags(tags, "device:"+deviceName, "device_name:"+deviceName, "operation:write")
			cext.sender(cext.aggSender.Rate, "containerd.blkio.service_recursive_bytes", deviceStats.WriteBytes, writeDeviceTags)
			cext.sender(cext.aggSender.Rate, "containerd.blkio.serviced_recursive", deviceStats.WriteOperations, writeDeviceTags)
		}

		if len(containerStats.IO.Devices) == 0 {
			readTags := taggerUtils.ConcatenateStringTags(tags, "operation:read")
			cext.sender(cext.aggSender.Rate, "containerd.blkio.service_recursive_bytes", containerStats.IO.ReadBytes, readTags)
			cext.sender(cext.aggSender.Rate, "containerd.blkio.serviced_recursive", containerStats.IO.ReadOperations, readTags)

			writeTags := taggerUtils.ConcatenateStringTags(tags, "operation:write")
			cext.sender(cext.aggSender.Rate, "containerd.blkio.service_recursive_bytes", containerStats.IO.WriteBytes, writeTags)
			cext.sender(cext.aggSender.Rate, "containerd.blkio.serviced_recursive", containerStats.IO.WriteOperations, writeTags)
		}
	}
}

// PostProcess is called once during each check run, after all calls to `Process`
func (cext *containerdCustomMetricsExtension) PostProcess() {
	// Nothing to do here
}
