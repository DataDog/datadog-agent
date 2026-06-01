// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Network Config Management (NCM). Each PipelineDesc returned by Descs() is
// contributed to the event platform forwarder via the "ep_pipeline_descs" fx
// group, so the NDM integrations team owns this configuration rather than the
// logs agent team.
package eventplatform

// team: ndm-integrations

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Descs returns the pipeline description for the Network Config Management event type.
func Descs(cfg pkgconfigmodel.Reader) []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeNetworkConfigManagement,
			Category:                      "Network Config Management",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "network_devices.config_management.forwarder.",
			HostnameEndpointPrefix:        "ndm-intake.",
			IntakeTrackType:               "ndmconfig",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    cfg.GetInt("network_devices.config_management.forwarder.batch_max_content_size"),
			DefaultBatchMaxSize:           cfg.GetInt("network_devices.config_management.forwarder.batch_max_size"),
			DefaultInputChanSize:          cfg.GetInt("network_devices.config_management.forwarder.input_chan_size"),
		},
	}
}
