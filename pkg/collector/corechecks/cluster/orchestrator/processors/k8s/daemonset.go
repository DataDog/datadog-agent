// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// DaemonSetHandlers implements the Handlers interface for Kubernetes DaemonSets.
type DaemonSetHandlers struct {
	common.BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.DaemonSet)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *DaemonSetHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.DaemonSet, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.DaemonSet))
	}

	return &model.CollectorDaemonSet{
		ClusterName: pctx.Cfg.KubeClusterName,
		ClusterId:   pctx.ClusterID,
		GroupId:     pctx.MsgGroupID,
		GroupSize:   int32(groupSize),
		DaemonSets:  models,
		Tags:        append(pctx.Cfg.ExtraTags, pctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*appsv1.DaemonSet)
	return k8sTransformers.ExtractDaemonSet(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*appsv1.DaemonSet)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*appsv1.DaemonSet).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*appsv1.DaemonSet).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *DaemonSetHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*appsv1.DaemonSet)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *DaemonSetHandlers) ScrubBeforeMarshalling(ctx processors.ProcessorContext, resource interface{}) {
	pctx := ctx.(*processors.K8sProcessorContext)
	r := resource.(*appsv1.DaemonSet)
	if pctx.Cfg.IsScrubbingEnabled {
		redact.ScrubPodTemplateSpec(&r.Spec.Template, pctx.Cfg.Scrubber)
	}
}
