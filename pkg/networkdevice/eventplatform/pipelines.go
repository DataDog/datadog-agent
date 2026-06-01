// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Network Device Monitoring event-platform pipeline descriptors.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Pipelines returns the Network Device Monitoring event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeNetworkDevicesMetadata,
			Category:                      "NDM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "network_devices.metadata.",
			HostnameEndpointPrefix:        "ndm-intake.",
			IntakeTrackType:               "ndm",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
