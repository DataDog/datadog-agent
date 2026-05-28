// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Software Inventory (EUDM). Each PipelineDesc returned by Descs() is
// contributed to the event platform forwarder via the "ep_pipeline_descs" fx
// group, so the Windows Products team owns this configuration rather than the
// logs agent team.
package eventplatform

// team: windows-products

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Descs returns the pipeline description for Software Inventory when the feature
// is enabled. Returns an empty slice when software_inventory.enabled is false.
func Descs() []eventplatform.PipelineDesc {
	if !pkgconfigsetup.Datadog().GetBool("software_inventory.enabled") {
		return nil
	}
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeSoftwareInventory,
			Category:                      "EUDM",
			ContentType:                   logshttp.JSONContentType,
			EndpointsConfigPrefix:         "software_inventory.forwarder.",
			HostnameEndpointPrefix:        "softinv-intake.",
			IntakeTrackType:               "softinv",
			DefaultBatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}
}
