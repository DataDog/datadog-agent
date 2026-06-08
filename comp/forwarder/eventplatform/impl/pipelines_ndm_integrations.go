// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func getNDMIntegrationsPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeNetworkDevicesNetFlow,
			category:                      "NDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_devices.netflow.forwarder.",
			hostnameEndpointPrefix:        "ndmflow-intake.",
			intakeTrackType:               "ndmflow",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			// Each NetFlow flow is about 500 bytes, we could fit ~10k is the default 5Mb content size. However,
			// this is also directly tied to the amount of work we need to do atomically in our event processing code to add enrichments.
			// Let's limit this size to 250 events, there will be some increased overhead since more packets will need to be sent.
			defaultBatchMaxSize: 250,
			// High input chan is needed to handle high number of flows being flushed by NetFlow Server every 10s
			// Customers might need to set `network_devices.forwarder.input_chan_size` to higher value if flows are dropped
			// due to input channel being full.
			// TODO: A possible better solution is to make SendEventPlatformEvent blocking when input chan is full and avoid
			//   dropping events. This can't be done right now due to SendEventPlatformEvent being called by
			//   aggregator loop, making SendEventPlatformEvent blocking might slow down other type of data handled
			//   by aggregator.
			defaultInputChanSize: 10000,
		},
		{
			eventType:                     eventplatform.EventTypeNetworkConfigManagement,
			category:                      "Network Config Management",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_devices.config_management.forwarder.",
			hostnameEndpointPrefix:        "ndm-intake.",
			intakeTrackType:               "ndmconfig",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
