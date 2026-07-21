// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func getNetworkPathPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeNetworkPath,
			category:                      "Network Path",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_path.forwarder.",
			hostnameEndpointPrefix:        "netpath-intake.",
			intakeTrackType:               "netpath",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
