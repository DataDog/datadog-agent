// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// container integrations: container lifecycle events, container images, and
// container SBOMs. Each PipelineDesc returned by Descs() is contributed to the
// event platform forwarder via the "ep_pipeline_descs" fx group, so the
// container integrations team owns this configuration rather than the logs
// agent team.
package eventplatform

// team: container-integrations

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Descs returns the pipeline descriptions for all container integration event types
// (lifecycle, images, SBOM).
func Descs() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     eventplatform.EventTypeContainerLifecycle,
			Category:                      "Container",
			ContentType:                   logshttp.ProtobufContentType,
			EndpointsConfigPrefix:         "container_lifecycle.",
			HostnameEndpointPrefix:        "contlcycle-intake.",
			IntakeTrackType:               "contlcycle",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			EventType:                     eventplatform.EventTypeContainerImages,
			Category:                      "Container",
			ContentType:                   logshttp.ProtobufContentType,
			EndpointsConfigPrefix:         "container_image.",
			HostnameEndpointPrefix:        "contimage-intake.",
			IntakeTrackType:               "contimage",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			EventType:   eventplatform.EventTypeContainerSBOM,
			Category:    "SBOM",
			ContentType: logshttp.ProtobufContentType,
			// co-owned with @DataDog/agent-security
			EndpointsConfigPrefix:         "sbom.",
			HostnameEndpointPrefix:        "sbom-intake.",
			IntakeTrackType:               "sbom",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// on every periodic refresh, we re-send all the SBOMs for all the
			// container images in the workloadmeta store. This can be a lot of
			// payloads at once, so we need a large input channel size to avoid dropping.
			DefaultInputChanSize: 1000,
		},
	}
}
