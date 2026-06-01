// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventmanagement contains Event Management event-platform pipeline descriptors.
package eventmanagement

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

// Pipelines returns the Event Management event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                      eventplatform.EventTypeEventManagement,
			Category:                       "Event Management",
			ContentType:                    eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:          "event_management.forwarder.",
			HostnameEndpointPrefix:         "event-management-intake.",
			IntakeTrackType:                "events",
			DefaultBatchMaxConcurrentSend:  eventplatform.DefaultBatchMaxConcurrentSend,
			DefaultBatchMaxContentSize:     eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:            eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:           eventplatform.DefaultInputChanSize,
			UseStreamStrategy:              true,
			SkipConnectivityDiagnose:       true,
			SkipConnectivityDiagnoseReason: "event-management-intake does not support the empty payload sent by diagnose",
		},
	}
}
