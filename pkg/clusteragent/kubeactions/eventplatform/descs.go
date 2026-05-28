// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// Kubernetes Actions. Each PipelineDesc returned by Descs() is contributed to
// the event platform forwarder via the "ep_pipeline_descs" fx group, so the
// container integrations team owns this configuration rather than the logs
// agent team.
package eventplatform

// team: container-integrations

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Descs returns the pipeline description for Kubernetes Actions when the feature
// is enabled. Returns an empty slice when kubeactions.enabled is false.
func Descs() []eventplatform.PipelineDesc {
	if !pkgconfigsetup.Datadog().GetBool("kubeactions.enabled") {
		return nil
	}
	desc := eventplatform.PipelineDesc{
		EventType:                     eventplatform.EventTypeKubeActions,
		Category:                      "Kubernetes Actions",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "kubeactions.forwarder.",
		HostnameEndpointPrefix:        "kubeops-intake.",
		IntakeTrackType:               "kubeactions",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}
	// TODO(kubeactions): Remove this log once EVP intake is stable
	log.Infof("[KubeActions] EVP pipeline registered: host_prefix=%s, track_type=%s, v2_api=%v",
		desc.HostnameEndpointPrefix,
		desc.IntakeTrackType,
		pkgconfigsetup.Datadog().GetBool("kubeactions.forwarder.use_v2_api"))
	return []eventplatform.PipelineDesc{desc}
}
