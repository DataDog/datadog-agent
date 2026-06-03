// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
)

func getSoftwareInventoryPipelines() []passthroughPipelineDesc {
	if !isSoftwareInventoryEnabled() {
		return nil
	}
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeSoftwareInventory,
			category:                      "EUDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "software_inventory.forwarder.",
			hostnameEndpointPrefix:        "softinv-intake.",
			intakeTrackType:               "softinv",
			defaultBatchMaxConcurrentSend: epfDefaultBatchMaxConcurrentSend,
			defaultBatchMaxContentSize:    epfDefaultBatchMaxContentSize,
			defaultBatchMaxSize:           epfDefaultBatchMaxSize,
			defaultInputChanSize:          epfDefaultInputChanSize,
		},
	}
}
