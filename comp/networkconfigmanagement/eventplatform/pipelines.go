// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Network Config Management event-platform pipeline descriptors.
package eventplatform

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

// Pipelines returns the Network Config Management event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeNetworkConfigManagement,
			Category:                      "Network Config Management",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "network_devices.config_management.forwarder.",
			HostnameEndpointPrefix:        "ndm-intake.",
			IntakeTrackType:               "ndmconfig",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:          eventplatform.DefaultInputChanSize,
		},
	}
}
