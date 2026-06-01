// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Synthetics event-platform pipeline descriptors.
package eventplatform

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

// Pipelines returns the Synthetics event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeSynthetics,
			Category:                      "Synthetics",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "synthetics.forwarder.",
			HostnameEndpointPrefix:        "http-synthetics.",
			IntakeTrackType:               "synthetics",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:          eventplatform.DefaultInputChanSize,
		},
	}
}
