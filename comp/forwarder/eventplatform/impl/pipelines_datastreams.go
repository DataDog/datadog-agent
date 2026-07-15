// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const eventTypeDataStreamsMessage = "data-streams-message"

func getDataStreamsPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventTypeDataStreamsMessage,
			category:                      "Data Streams",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "data_streams.forwarder.",
			hostnameEndpointPrefix:        "trace.agent.",
			intakeTrackType:               "data_streams_messages",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
