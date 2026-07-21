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

func getEventManagementPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeEventManagement,
			category:                      "Event Management",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "event_management.forwarder.",
			hostnameEndpointPrefix:        "event-management-intake.",
			intakeTrackType:               "events",
			defaultBatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
			//nolint:misspell
			// TODO(ECT-4272): event-management-intake does not support batching/array, must send one event at a time
			useStreamStrategy: true,
		},
	}
}
