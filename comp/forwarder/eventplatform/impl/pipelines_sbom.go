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

func getSBOMPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeContainerSBOM,
			category:                      "SBOM",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "sbom.",
			hostnameEndpointPrefix:        "sbom-intake.",
			intakeTrackType:               "sbom",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// on every periodic refresh, we re-send all the SBOMs for all the
			// container images in the workloadmeta store. This can be a lot of
			// payloads at once, so we need a large input channel size to avoid dropping
			defaultInputChanSize: 1000,
		},
	}
}
