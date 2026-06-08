// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const eventTypeDoQueryResults = "do-query-results"

func getDataObservabilityPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventTypeDoQueryResults,
			category:                      "DO",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "data_observability.forwarder.",
			hostnameEndpointPrefix:        "data-obs-intake.",
			intakeTrackType:               "query-actions",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          500,
		},
	}
}
