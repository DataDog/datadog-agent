// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PersistentVolumeHandlers implements the Handlers interface for Kubernetes PersistentVolumes.
type PersistentVolumeHandlers struct {
	BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
func (h *PersistentVolumeHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.PersistentVolume)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *PersistentVolumeHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.PersistentVolume, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.PersistentVolume))
	}

	return &model.CollectorPersistentVolume{
		ClusterName:       ctx.Cfg.KubeClusterName,
		ClusterId:         ctx.ClusterID,
		GroupId:           ctx.MsgGroupID,
		GroupSize:         int32(groupSize),
		PersistentVolumes: models,
		Tags:              append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *PersistentVolumeHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*corev1.PersistentVolume)
	return k8sTransformers.ExtractPersistentVolume(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *PersistentVolumeHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*corev1.PersistentVolume)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *PersistentVolumeHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*corev1.PersistentVolume).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *PersistentVolumeHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*corev1.PersistentVolume).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *PersistentVolumeHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.PersistentVolume)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}
