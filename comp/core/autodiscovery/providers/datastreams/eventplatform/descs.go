// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Data Streams monitoring. Each PipelineDesc returned by Descs() is contributed
// to the event platform forwarder via the "ep_pipeline_descs" fx group, so the
// Data Streams team owns this configuration rather than the logs agent team.
package eventplatform

// team: data-streams-monitoring

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Descs returns the pipeline description for the Data Streams event type.
func Descs(cfg pkgconfigmodel.Reader) []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeDataStreamsMessage,
			Category:                      "Data Streams",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "data_streams.forwarder.",
			HostnameEndpointPrefix:        "trace.agent.",
			IntakeTrackType:               "data_streams_messages",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    cfg.GetInt("data_streams.forwarder.batch_max_content_size"),
			DefaultBatchMaxSize:           cfg.GetInt("data_streams.forwarder.batch_max_size"),
			DefaultInputChanSize:          cfg.GetInt("data_streams.forwarder.input_chan_size"),
		},
	}
}
