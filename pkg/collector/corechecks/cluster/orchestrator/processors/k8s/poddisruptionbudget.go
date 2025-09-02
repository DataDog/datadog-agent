// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PodDisruptionBudgetHandlers implements the Handlers interface for Kubernetes NetworkPolicy.
type PodDisruptionBudgetHandlers struct {
	common.BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
func (h *PodDisruptionBudgetHandlers) AfterMarshalling(_ processors.ProcessorContext, _, _ interface{}, _ []byte) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *PodDisruptionBudgetHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*policyv1.PodDisruptionBudget)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *PodDisruptionBudgetHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.PodDisruptionBudget, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.PodDisruptionBudget))
	}

	return &model.CollectorPodDisruptionBudget{
		ClusterName:          pctx.Cfg.KubeClusterName,
		ClusterId:            pctx.ClusterID,
		GroupId:              pctx.MsgGroupID,
		GroupSize:            int32(groupSize),
		PodDisruptionBudgets: models,
		Tags:                 util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion:         ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *PodDisruptionBudgetHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*policyv1.PodDisruptionBudget)
	return k8sTransformers.ExtractPodDisruptionBudget(ctx, r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *PodDisruptionBudgetHandlers) ResourceList(_ processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*policyv1.PodDisruptionBudget)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource.DeepCopy())
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *PodDisruptionBudgetHandlers) ResourceUID(_ processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*policyv1.PodDisruptionBudget).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *PodDisruptionBudgetHandlers) ResourceVersion(_ processors.ProcessorContext, resource, _ interface{}) string {
	return resource.(*policyv1.PodDisruptionBudget).ResourceVersion
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *PodDisruptionBudgetHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.PodDisruptionBudget)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *PodDisruptionBudgetHandlers) ScrubBeforeExtraction(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*policyv1.PodDisruptionBudget)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *PodDisruptionBudgetHandlers) ScrubBeforeMarshalling(_ processors.ProcessorContext, _ interface{}) {
}
