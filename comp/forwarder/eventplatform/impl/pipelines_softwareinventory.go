// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

func getSoftwareInventoryPipelines() []passthroughPipelineDesc {
	if !pkgconfigsetup.Datadog().GetBool("software_inventory.enabled") {
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
			defaultBatchMaxConcurrentSend: constants.DefaultBatchMaxConcurrentSend,
			defaultBatchMaxContentSize:    constants.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           constants.DefaultBatchMaxSize,
			defaultInputChanSize:          constants.DefaultInputChanSize,
		},
	}
}
