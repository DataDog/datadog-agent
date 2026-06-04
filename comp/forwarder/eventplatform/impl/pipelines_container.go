// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getContainerPipelines() []passthroughPipelineDesc {
	descs := []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeContainerLifecycle,
			category:                      "Container",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "container_lifecycle.",
			hostnameEndpointPrefix:        "contlcycle-intake.",
			intakeTrackType:               "contlcycle",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    epfDefaultBatchMaxContentSize,
			defaultBatchMaxSize:           epfDefaultBatchMaxSize,
			defaultInputChanSize:          epfDefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeContainerImages,
			category:                      "Container",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "container_image.",
			hostnameEndpointPrefix:        "contimage-intake.",
			intakeTrackType:               "contimage",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    epfDefaultBatchMaxContentSize,
			defaultBatchMaxSize:           epfDefaultBatchMaxSize,
			defaultInputChanSize:          epfDefaultInputChanSize,
		},
	}

	if isKubeActionsEnabled() {
		kubeactionsPipeline := passthroughPipelineDesc{
			eventType:                     eventplatform.EventTypeKubeActions,
			category:                      "Kubernetes Actions",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "kubeactions.forwarder.",
			hostnameEndpointPrefix:        "kubeops-intake.",
			intakeTrackType:               "kubeactions",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    epfDefaultBatchMaxContentSize,
			defaultBatchMaxSize:           epfDefaultBatchMaxSize,
			defaultInputChanSize:          epfDefaultInputChanSize,
		}
		descs = append(descs, kubeactionsPipeline)
		// TODO(kubeactions): Remove this log once EVP intake is stable
		log.Infof("[KubeActions] EVP pipeline registered: host_prefix=%s, track_type=%s, v2_api=%v",
			kubeactionsPipeline.hostnameEndpointPrefix,
			kubeactionsPipeline.intakeTrackType,
			kubeActionsForwarderUseV2API())
	}

	return descs
}
