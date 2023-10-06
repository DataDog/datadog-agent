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
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

// VerticalPodAutoscalerHandlers implements the Handlers interface for Kuberenetes VPAs
type VerticalPodAutoscalerHandlers struct {
	BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
func (h *VerticalPodAutoscalerHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.VerticalPodAutoscaler)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *VerticalPodAutoscalerHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.VerticalPodAutoscaler, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.VerticalPodAutoscaler))
	}

	return &model.CollectorVerticalPodAutoscaler{
		ClusterName:            ctx.Cfg.KubeClusterName,
		ClusterId:              ctx.ClusterID,
		GroupId:                ctx.MsgGroupID,
		GroupSize:              int32(groupSize),
		VerticalPodAutoscalers: models,
		Tags:                   append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *VerticalPodAutoscalerHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (verticalPodAutoscalerModel interface{}) {
	r := resource.(*v1.VerticalPodAutoscaler)
	return k8sTransformers.ExtractVerticalPodAutoscaler(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *VerticalPodAutoscalerHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*v1.VerticalPodAutoscaler)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *VerticalPodAutoscalerHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*v1.VerticalPodAutoscaler).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *VerticalPodAutoscalerHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*v1.VerticalPodAutoscaler).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *VerticalPodAutoscalerHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*v1.VerticalPodAutoscaler)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}
