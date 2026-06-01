// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains NetFlow event-platform pipeline descriptors.
package eventplatform

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

// Pipelines returns the NetFlow event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeNetworkDevicesNetFlow,
			Category:                      "NDM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "network_devices.netflow.forwarder.",
			HostnameEndpointPrefix:        "ndmflow-intake.",
			IntakeTrackType:               "ndmflow",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           250,
			DefaultInputChanSize:          10000,
		},
	}
}
