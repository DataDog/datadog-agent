// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Kubernetes Actions event-platform pipeline descriptors.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Pipelines returns the Kubernetes Actions event-platform pipelines.
func Pipelines(cfg model.Reader) []eventplatform.PipelineDesc {
	if !cfg.GetBool("kubeactions.enabled") {
		return nil
	}
	return []eventplatform.PipelineDesc{
		{
			EventType:                      eventplatform.EventTypeKubeActions,
			Category:                       "Kubernetes Actions",
			ContentType:                    eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:          "kubeactions.forwarder.",
			HostnameEndpointPrefix:         "kubeops-intake.",
			IntakeTrackType:                "kubeactions",
			DefaultBatchMaxConcurrentSend:  10,
			DefaultBatchMaxContentSize:     pkgconfigsetup.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:            pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:           pkgconfigsetup.DefaultInputChanSize,
			SkipConnectivityDiagnose:       true,
			SkipConnectivityDiagnoseReason: "kubeactions-intake does not support the empty payload sent by diagnose",
		},
	}
}
