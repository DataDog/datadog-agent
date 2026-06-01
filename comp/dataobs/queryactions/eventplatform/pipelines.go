// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Data Observability event-platform pipeline descriptors.
package eventplatform

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

const EventTypeDoQueryResults = "do-query-results"

// Pipelines returns the Data Observability event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                      EventTypeDoQueryResults,
			Category:                       "DO",
			ContentType:                    eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:          "data_observability.forwarder.",
			HostnameEndpointPrefix:         "data-obs-intake.",
			IntakeTrackType:                "query-actions",
			DefaultBatchMaxConcurrentSend:  10,
			DefaultBatchMaxContentSize:     20e6,
			DefaultBatchMaxSize:            eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:           500,
			SkipConnectivityDiagnose:       true,
			SkipConnectivityDiagnoseReason: "data-obs-intake query-actions does not support the empty payload sent by diagnose",
		},
	}
}
