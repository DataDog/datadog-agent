// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterRoleHandlers implements the Handlers interface for Kubernetes ClusterRoles.
type ClusterRoleHandlers struct {
	common.BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.ClusterRole)
	m.Yaml = yaml
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*rbacv1.ClusterRole)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *ClusterRoleHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.ClusterRole, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.ClusterRole))
	}

	return &model.CollectorClusterRole{
		ClusterName:  pctx.Cfg.KubeClusterName,
		ClusterId:    pctx.ClusterID,
		GroupId:      pctx.MsgGroupID,
		GroupSize:    int32(groupSize),
		ClusterRoles: models,
		Tags:         util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion: ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*rbacv1.ClusterRole)
	return k8sTransformers.ExtractClusterRole(ctx, r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*rbacv1.ClusterRole)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource.DeepCopy())
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*rbacv1.ClusterRole).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*rbacv1.ClusterRole).ResourceVersion
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.ClusterRole)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*rbacv1.ClusterRole)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}
