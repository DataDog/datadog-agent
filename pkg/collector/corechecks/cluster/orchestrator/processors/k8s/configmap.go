// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

// ConfigMapHandlers implements the Handlers interface for Kubernetes ConfigMaps.
// ConfigMap is manifest-only (IsMetadataProducer: false): no structured metadata model is
// produced or forwarded. Resources are collected through a metadata-only informer, so
// Data and BinaryData are never fetched and the manifest only contains the object metadata.
type ConfigMapHandlers struct {
	common.BaseHandlers
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

// BeforeMarshalling is a handler called before resource marshalling.
// Sets Kind and APIVersion on the object, which the metadata API reports as
// PartialObjectMetadata, so the manifest is emitted as a ConfigMap.
//
//nolint:revive
func (h *ConfigMapHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, _ interface{}) (skip bool) {
	r := resource.(*metav1.PartialObjectMetadata)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
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

// ResourceList converts the raw list to a slice of generic interfaces.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceList(_ processors.ProcessorContext, list interface{}) []interface{} {
	resourceList := list.([]*metav1.PartialObjectMetadata)
	resources := make([]interface{}, 0, len(resourceList))
	for _, r := range resourceList {
		resources = append(resources, r)
	}
	return resources
}

// CloneResource returns a deep copy of the ConfigMap so mutations during scrubbing
// do not affect the informer cache.
//
//nolint:revive
func (h *ConfigMapHandlers) CloneResource(resource interface{}) interface{} {
	return resource.(*metav1.PartialObjectMetadata).DeepCopy()
}

// ResourceVersionFromRaw returns the resource version without requiring model extraction.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceVersionFromRaw(_ processors.ProcessorContext, resource interface{}) string {
	return resource.(*metav1.PartialObjectMetadata).ResourceVersion
}

// ResourceUID returns the UID of the ConfigMap.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceUID(_ processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*metav1.PartialObjectMetadata).UID
}

// ResourceVersion returns the resource version of the ConfigMap.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceVersion(_ processors.ProcessorContext, resource, _ interface{}) string {
	return resource.(*metav1.PartialObjectMetadata).ResourceVersion
}

// ScrubBeforeExtraction redacts sensitive annotation and label keys before the resource is processed.
//
//nolint:revive
func (h *ConfigMapHandlers) ScrubBeforeExtraction(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*metav1.PartialObjectMetadata)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}

// ScrubBeforeMarshalling strips ManagedFields so that field-manager history is never
// included in the emitted manifest.
//
//nolint:revive
func (h *ConfigMapHandlers) ScrubBeforeMarshalling(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*metav1.PartialObjectMetadata)
	r.ManagedFields = nil
}
