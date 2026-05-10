// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/redact"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterRoleBindingHandlers implements the Handlers interface for Kubernetes ClusteRoleBindings.
type ClusterRoleBindingHandlers struct {
	common.BaseHandlers
	tagger tagger.Component
}

// NewClusterRoleBindingHandlers creates a new ClusterRoleBindingHandlers.
func NewClusterRoleBindingHandlers(tagger tagger.Component) *ClusterRoleBindingHandlers {
	return &ClusterRoleBindingHandlers{tagger: tagger}
}

// BeforeCacheCheck is a handler called before cache lookup.
//
//nolint:revive
func (h *ClusterRoleBindingHandlers) BeforeCacheCheck(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	m := resourceModel.(*model.ClusterRoleBinding)

	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), "", r.Name)),
	)
	taggerTags, err := h.tagger.Tag(entityID, taggertypes.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for clusterrolebinding %s: %s", r.Name, err)
		return
	}

	m.Tags = append(m.Tags, taggerTags...)
	return
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.ClusterRoleBinding)
	m.Yaml = yaml
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *ClusterRoleBindingHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.ClusterRoleBinding, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.ClusterRoleBinding))
	}

	return &model.CollectorClusterRoleBinding{
		ClusterName:         pctx.Cfg.KubeClusterName,
		ClusterId:           pctx.ClusterID,
		GroupId:             pctx.MsgGroupID,
		GroupSize:           int32(groupSize),
		ClusterRoleBindings: models,
		Tags:                util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion:        ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	return k8sTransformers.ExtractClusterRoleBinding(ctx, r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*rbacv1.ClusterRoleBinding)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource.DeepCopy())
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*rbacv1.ClusterRoleBinding).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*rbacv1.ClusterRoleBinding).ResourceVersion
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.ClusterRoleBinding)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *ClusterRoleBindingHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*rbacv1.ClusterRoleBinding)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}
