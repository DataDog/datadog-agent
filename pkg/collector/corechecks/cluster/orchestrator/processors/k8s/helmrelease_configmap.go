// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

// HelmReleaseConfigMapHandlers implements the Handlers interface for Helm releases stored in Kubernetes ConfigMaps.
// It is manifest-only (IsMetadataProducer: false): the release payload is sent as is, without decoding it.
type HelmReleaseConfigMapHandlers struct {
	common.BaseHandlers
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

// BeforeMarshalling is a handler called before resource marshalling.
// Sets Kind and APIVersion on the object, which the Kubernetes API omits on typed responses.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, _ interface{}) (skip bool) {
	r := resource.(*corev1.ConfigMap)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
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

// ResourceList converts the raw list to a slice of generic interfaces.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ResourceList(_ processors.ProcessorContext, list interface{}) []interface{} {
	resourceList := list.([]*corev1.ConfigMap)
	resources := make([]interface{}, 0, len(resourceList))
	for _, r := range resourceList {
		resources = append(resources, r)
	}
	return resources
}

// CloneResource returns a deep copy of the ConfigMap so mutations during scrubbing
// do not affect the informer cache. The release payload string is shared, not copied.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) CloneResource(resource interface{}) interface{} {
	return resource.(*corev1.ConfigMap).DeepCopy()
}

// ResourceVersionFromRaw returns the resource version without requiring model extraction.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ResourceVersionFromRaw(_ processors.ProcessorContext, resource interface{}) string {
	return resource.(*corev1.ConfigMap).ResourceVersion
}

// ResourceUID returns the UID of the ConfigMap.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ResourceUID(_ processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*corev1.ConfigMap).UID
}

// ResourceVersion returns the resource version of the ConfigMap.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ResourceVersion(_ processors.ProcessorContext, resource, _ interface{}) string {
	return resource.(*corev1.ConfigMap).ResourceVersion
}

// ScrubBeforeExtraction redacts sensitive annotation and label keys before the resource is processed.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ScrubBeforeExtraction(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.ConfigMap)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}

// ScrubBeforeMarshalling strips ManagedFields so that field-manager history is never
// included in the emitted manifest.
//
//nolint:revive
func (h *HelmReleaseConfigMapHandlers) ScrubBeforeMarshalling(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.ConfigMap)
	r.ManagedFields = nil
}
