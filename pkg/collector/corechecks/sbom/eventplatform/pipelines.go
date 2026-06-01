// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains SBOM event-platform pipeline descriptors.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Pipelines returns the SBOM event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeContainerSBOM,
			Category:                      "SBOM",
			ContentType:                   eventplatform.ContentTypeProtobuf,
			EndpointsConfigPrefix:         "sbom.",
			HostnameEndpointPrefix:        "sbom-intake.",
			IntakeTrackType:               "sbom",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          1000,
		},
	}
}
