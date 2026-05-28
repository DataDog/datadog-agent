// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// network device monitoring (NDM). Each PipelineDesc returned by Descs() is
// contributed to the event platform forwarder via the "ep_pipeline_descs" fx
// group, so the NDM team owns this configuration rather than the logs agent team.
//
// Scope: this package covers the NDM-core surface area (metadata, SNMP traps,
// NetFlow). The Network Path and Network Config Management pipelines have
// separate owners and should live in their own team-owned packages.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// team: network-device-monitoring-core

// Descs returns the pipeline descriptions for the NDM-owned event types.
func Descs() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeNetworkDevicesMetadata,
			Category:                      "NDM",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "network_devices.metadata.",
			HostnameEndpointPrefix:        "ndm-intake.",
			IntakeTrackType:               "ndm",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			EventType:                     eventplatform.EventTypeSnmpTraps,
			Category:                      "NDM",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
			HostnameEndpointPrefix:        "snmp-traps-intake.",
			IntakeTrackType:               "ndmtraps",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			EventType:                     eventplatform.EventTypeNetworkDevicesNetFlow,
			Category:                      "NDM",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "network_devices.netflow.forwarder.",
			HostnameEndpointPrefix:        "ndmflow-intake.",
			IntakeTrackType:               "ndmflow",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,

			// Each NetFlow flow is about 500 bytes, we could fit ~10k is the default 5Mb content size. However,
			// this is also directly tied to the amount of work we need to do atomically in our event processing code to add enrichments.
			// Let's limit this size to 250 events, there will be some increased overhead since more packets will need to be sent.
			DefaultBatchMaxSize: 250,
			// High input chan is needed to handle high number of flows being flushed by NetFlow Server every 10s
			// Customers might need to set `network_devices.forwarder.input_chan_size` to higher value if flows are dropped
			// due to input channel being full.
			// TODO: A possible better solution is to make SendEventPlatformEvent blocking when input chan is full and avoid
			//   dropping events. This can't be done right now due to SendEventPlatformEvent being called by
			//   aggregator loop, making SendEventPlatformEvent blocking might slow down other type of data handled
			//   by aggregator.
			DefaultInputChanSize: 10000,
		},
	}
}
