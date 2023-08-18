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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// DeploymentHandlers implements the Handlers interface for Kubernetes Deployments.
type DeploymentHandlers struct {
	BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
func (h *DeploymentHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.Deployment)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *DeploymentHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.Deployment, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.Deployment))
	}

	return &model.CollectorDeployment{
		ClusterName: ctx.Cfg.KubeClusterName,
		ClusterId:   ctx.ClusterID,
		GroupId:     ctx.MsgGroupID,
		GroupSize:   int32(groupSize),
		Deployments: models,
		Tags:        append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *DeploymentHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*appsv1.Deployment)
	return k8sTransformers.ExtractDeployment(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *DeploymentHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*appsv1.Deployment)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *DeploymentHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*appsv1.Deployment).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *DeploymentHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*appsv1.Deployment).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *DeploymentHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*appsv1.Deployment)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *DeploymentHandlers) ScrubBeforeMarshalling(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*appsv1.Deployment)
	if ctx.Cfg.IsScrubbingEnabled {
		redact.ScrubPodTemplateSpec(&r.Spec.Template, ctx.Cfg.Scrubber)
	}
}
