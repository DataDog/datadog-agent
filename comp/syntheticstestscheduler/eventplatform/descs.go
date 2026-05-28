// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Synthetics. Each PipelineDesc returned by Descs() is contributed to the
// event platform forwarder via the "ep_pipeline_descs" fx group, so the
// Synthetics team owns this configuration rather than the logs agent team.
package eventplatform

// team: synthetics-executing

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Descs returns the pipeline description for the Synthetics event type.
func Descs() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeSynthetics,
			Category:                      "Synthetics",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "synthetics.forwarder.",
			HostnameEndpointPrefix:        "http-synthetics.",
			IntakeTrackType:               "synthetics",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
