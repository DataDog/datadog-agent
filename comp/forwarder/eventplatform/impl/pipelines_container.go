// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeContainerImages,
			category:                      "Container",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "container_image.",
			hostnameEndpointPrefix:        "contimage-intake.",
			intakeTrackType:               "contimage",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}

	if pkgconfigsetup.Datadog().GetBool("kubeactions.enabled") {
		kubeactionsPipeline := passthroughPipelineDesc{
			eventType:                     eventplatform.EventTypeKubeActions,
			category:                      "Kubernetes Actions",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "kubeactions.forwarder.",
			hostnameEndpointPrefix:        "kubeops-intake.",
			intakeTrackType:               "kubeactions",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		}
		descs = append(descs, kubeactionsPipeline)
		// TODO(kubeactions): Remove this log once EVP intake is stable
		log.Infof("[KubeActions] EVP pipeline registered: host_prefix=%s, track_type=%s, v2_api=%v",
			kubeactionsPipeline.hostnameEndpointPrefix,
			kubeactionsPipeline.intakeTrackType,
			pkgconfigsetup.Datadog().GetBool("kubeactions.forwarder.use_v2_api"))
	}

	return descs
}
