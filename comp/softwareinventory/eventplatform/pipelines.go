// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Software Inventory event-platform pipeline descriptors.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Pipelines returns the Software Inventory event-platform pipelines.
func Pipelines(cfg model.Reader) []eventplatform.PipelineDesc {
	if !cfg.GetBool("software_inventory.enabled") {
		return nil
	}
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeSoftwareInventory,
			Category:                      "EUDM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "software_inventory.forwarder.",
			HostnameEndpointPrefix:        "softinv-intake.",
			IntakeTrackType:               "softinv",
			DefaultBatchMaxConcurrentSend: eventplatform.DefaultBatchMaxConcurrentSend,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:          eventplatform.DefaultInputChanSize,
		},
	}
}
