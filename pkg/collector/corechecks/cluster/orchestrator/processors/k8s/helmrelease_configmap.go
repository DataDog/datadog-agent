// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	corev1 "k8s.io/api/core/v1"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
)

// HelmReleaseConfigMapHandlers implements the Handlers interface for Helm releases stored in Kubernetes ConfigMaps.
// It is manifest-only (IsMetadataProducer: false): the release payload is sent as is, without decoding it.
type HelmReleaseConfigMapHandlers struct {
	common.TypedHandlers[*corev1.ConfigMap]
}

// NewHelmReleaseConfigMapHandlers creates a new HelmReleaseConfigMapHandlers.
func NewHelmReleaseConfigMapHandlers() *HelmReleaseConfigMapHandlers {
	return &HelmReleaseConfigMapHandlers{}
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) AfterMarshalling(_ processors.ProcessorContext, _, _ interface{}, _ []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of extracted resources.
// Helm releases are manifest-only so no metadata message body is ever sent.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) BuildMessageBody(_ processors.ProcessorContext, _ []interface{}, _ int) model.MessageBody {
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
// Helm releases are manifest-only; no structured model is produced.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ExtractResource(_ processors.ProcessorContext, _ interface{}) interface{} {
	return nil
}

// ScrubBeforeMarshalling strips ManagedFields so that field-manager history is never
// included in the emitted manifest.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ScrubBeforeMarshalling(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.ConfigMap)
	r.ManagedFields = nil
}
