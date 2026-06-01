// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains SNMP traps event-platform pipeline descriptors.
package eventplatform

import eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"

// Pipelines returns the SNMP traps event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeSnmpTraps,
			Category:                      "NDM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
			HostnameEndpointPrefix:        "snmp-traps-intake.",
			IntakeTrackType:               "ndmtraps",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:          eventplatform.DefaultInputChanSize,
		},
	}
}
