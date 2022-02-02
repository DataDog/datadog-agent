// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// K8sPersistentVolumeClaimHandlers implements the Handlers interface for Kubernetes PersistentVolumeClaims.
type K8sPersistentVolumeClaimHandlers struct{}

// AfterMarshalling is a handler called after resource marshalling.
func (h *K8sPersistentVolumeClaimHandlers) AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.PersistentVolumeClaim)
	m.Yaml = yaml
	return
}

// BeforeCacheCheck is a handler called before cache lookup.
func (h *K8sPersistentVolumeClaimHandlers) BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
func (h *K8sPersistentVolumeClaimHandlers) BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *K8sPersistentVolumeClaimHandlers) BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.PersistentVolumeClaim, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.PersistentVolumeClaim))
	}

	return &model.CollectorPersistentVolumeClaim{
		ClusterName:            ctx.Cfg.KubeClusterName,
		ClusterId:              ctx.ClusterID,
		GroupId:                ctx.MsgGroupID,
		GroupSize:              int32(groupSize),
		PersistentVolumeClaims: models,
		Tags:                   ctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *K8sPersistentVolumeClaimHandlers) ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*corev1.PersistentVolumeClaim)
	return transformers.ExtractK8sPersistentVolumeClaim(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *K8sPersistentVolumeClaimHandlers) ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*corev1.PersistentVolumeClaim)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *K8sPersistentVolumeClaimHandlers) ResourceUID(ctx *ProcessorContext, resource, resourceModel interface{}) types.UID {
	return resource.(*corev1.PersistentVolumeClaim).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *K8sPersistentVolumeClaimHandlers) ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*corev1.PersistentVolumeClaim).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *K8sPersistentVolumeClaimHandlers) ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*corev1.PersistentVolumeClaim)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *K8sPersistentVolumeClaimHandlers) ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{}) {
}
