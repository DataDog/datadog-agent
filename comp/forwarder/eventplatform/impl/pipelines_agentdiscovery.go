// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
)

func getAgentDiscoveryPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeAgentDiscovery,
			category:                      "Agent Discovery",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "config_files_discovery.forwarder.",
			hostnameEndpointPrefix:        "agentdiscovery-intake.",
			intakeTrackType:               "agentdiscovery",
			defaultBatchMaxConcurrentSend: 0,
			defaultBatchMaxContentSize:    5000000,
			defaultBatchMaxSize:           1000,
			defaultInputChanSize:          100,
		},
	}
}
