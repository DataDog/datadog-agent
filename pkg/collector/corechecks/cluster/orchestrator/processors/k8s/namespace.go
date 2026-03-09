// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	corev1 "k8s.io/api/core/v1"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/redact"

	"k8s.io/apimachinery/pkg/types"
)

// NamespaceHandlers implements the Handlers interface for Kubernetes Namespace.
type NamespaceHandlers struct {
	common.BaseHandlers
	tagger tagger.Component
}

// NewNamespaceHandlers creates a new NamespaceHandlers.
func NewNamespaceHandlers(tagger tagger.Component) *NamespaceHandlers {
	return &NamespaceHandlers{tagger: tagger}
}

// BeforeCacheCheck is a handler called before cache lookup.
//
//nolint:revive
func (h *NamespaceHandlers) BeforeCacheCheck(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*corev1.Namespace)
	m := resourceModel.(*model.Namespace)

	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), "", r.Name)),
	)
	taggerTags, err := h.tagger.Tag(entityID, taggertypes.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for namespace %s: %s", r.Name, err)
		return
	}

	m.Tags = append(m.Tags, taggerTags...)
	return
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.Namespace)
	m.Yaml = yaml
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*corev1.Namespace)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *NamespaceHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.Namespace, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.Namespace))
	}

	return &model.CollectorNamespace{
		ClusterName:  pctx.Cfg.KubeClusterName,
		ClusterId:    pctx.ClusterID,
		GroupId:      pctx.MsgGroupID,
		GroupSize:    int32(groupSize),
		Namespaces:   models,
		Tags:         util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion: ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (namespaceModel interface{}) {
	r := resource.(*corev1.Namespace)
	return k8sTransformers.ExtractNamespace(ctx, r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*corev1.Namespace)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource.DeepCopy())
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*corev1.Namespace).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*corev1.Namespace).ResourceVersion
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.Namespace)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NamespaceHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.Namespace)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}
