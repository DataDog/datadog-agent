// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Data Observability query actions. Each PipelineDesc returned by Descs() is
// contributed to the event platform forwarder via the "ep_pipeline_descs" fx
// group, so the Data Observability team owns this configuration rather than
// the logs agent team.
package eventplatform

// team: data-observability

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Descs returns the pipeline description for the Data Observability query results event type.
func Descs(cfg pkgconfigmodel.Reader) []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeDoQueryResults,
			Category:                      "DO",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "data_observability.forwarder.",
			HostnameEndpointPrefix:        "data-obs-intake.",
			IntakeTrackType:               "query-actions",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           cfg.GetInt("data_observability.forwarder.batch_max_size"),
			DefaultInputChanSize:          500,
		},
	}
}
