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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// K8sReplicaSetHandlers implements the Handlers interface for Kubernetes ReplicaSets.
type K8sReplicaSetHandlers struct{}

// AfterMarshalling is a handler called after resource marshalling.
func (h *K8sReplicaSetHandlers) AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.ReplicaSet)
	m.Yaml = yaml
	return
}

// BeforeCacheCheck is a handler called before cache lookup.
func (h *K8sReplicaSetHandlers) BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
func (h *K8sReplicaSetHandlers) BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *K8sReplicaSetHandlers) BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.ReplicaSet, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.ReplicaSet))
	}

	return &model.CollectorReplicaSet{
		ClusterName: ctx.Cfg.KubeClusterName,
		ClusterId:   ctx.ClusterID,
		GroupId:     ctx.MsgGroupID,
		GroupSize:   int32(groupSize),
		ReplicaSets: models,
		Tags:        ctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *K8sReplicaSetHandlers) ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*appsv1.ReplicaSet)
	return transformers.ExtractK8sReplicaSet(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *K8sReplicaSetHandlers) ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*appsv1.ReplicaSet)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *K8sReplicaSetHandlers) ResourceUID(ctx *ProcessorContext, resource, resourceModel interface{}) types.UID {
	return resource.(*appsv1.ReplicaSet).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *K8sReplicaSetHandlers) ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*appsv1.ReplicaSet).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *K8sReplicaSetHandlers) ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*appsv1.ReplicaSet)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *K8sReplicaSetHandlers) ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*appsv1.ReplicaSet)
	if ctx.Cfg.IsScrubbingEnabled {
		for c := 0; c < len(r.Spec.Template.Spec.InitContainers); c++ {
			redact.ScrubContainer(&r.Spec.Template.Spec.InitContainers[c], ctx.Cfg.Scrubber)
		}
		for c := 0; c < len(r.Spec.Template.Spec.Containers); c++ {
			redact.ScrubContainer(&r.Spec.Template.Spec.Containers[c], ctx.Cfg.Scrubber)
		}
	}
}
