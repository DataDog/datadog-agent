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

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterRoleBindingHandlers implements the Handlers interface for Kubernetes ClusteRoleBindings.
type ClusterRoleBindingHandlers struct {
	BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.ClusterRoleBinding)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *ClusterRoleBindingHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.ClusterRoleBinding, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.ClusterRoleBinding))
	}

	return &model.CollectorClusterRoleBinding{
		ClusterName:         ctx.Cfg.KubeClusterName,
		ClusterId:           ctx.ClusterID,
		GroupId:             ctx.MsgGroupID,
		GroupSize:           int32(groupSize),
		ClusterRoleBindings: models,
		Tags:                append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	return k8sTransformers.ExtractClusterRoleBinding(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*rbacv1.ClusterRoleBinding)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*rbacv1.ClusterRoleBinding).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*rbacv1.ClusterRoleBinding).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}
