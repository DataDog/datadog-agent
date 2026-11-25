// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
)

// ConfigMapHandlers implements the Handlers interface for Kubernetes ConfigMaps.
// ConfigMap is manifest-only (IsMetadataProducer: false): no structured metadata model is
// produced or forwarded. Resources are collected through a metadata-only informer, so
// Data and BinaryData are never fetched and the manifest only contains the object metadata.
type ConfigMapHandlers struct {
	common.TypedHandlers[*metav1.PartialObjectMetadata]
}

// NewConfigMapHandlers creates a new ConfigMapHandlers.
func NewConfigMapHandlers() *ConfigMapHandlers {
	return &ConfigMapHandlers{}
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive
func (h *ConfigMapHandlers) AfterMarshalling(_ processors.ProcessorContext, _, _ interface{}, _ []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of extracted resources.
// ConfigMap is manifest-only so no metadata message body is ever sent.
//
//nolint:revive
func (h *ConfigMapHandlers) BuildMessageBody(_ processors.ProcessorContext, _ []interface{}, _ int) model.MessageBody {
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
// ConfigMap is manifest-only; no structured model is produced.
//
//nolint:revive
func (h *ConfigMapHandlers) ExtractResource(_ processors.ProcessorContext, _ interface{}) interface{} {
	return nil
}

// ScrubBeforeMarshalling strips ManagedFields so that field-manager history is never
// included in the emitted manifest.
//
//nolint:revive
func (h *ConfigMapHandlers) ScrubBeforeMarshalling(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*metav1.PartialObjectMetadata)
	r.ManagedFields = nil
}
