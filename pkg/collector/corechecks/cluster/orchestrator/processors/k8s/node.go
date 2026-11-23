// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"

	corev1 "k8s.io/api/core/v1"
)

// NodeHandlers implements the Handlers interface for Kubernetes Nodes.
type NodeHandlers struct {
	common.TypedHandlers[*corev1.Node]
	tagger tagger.Component
}

// NewNodeHandlers creates a new NodeHandlers.
func NewNodeHandlers(tagger tagger.Component) *NodeHandlers {
	return &NodeHandlers{tagger: tagger}
}

// EnrichModel is a handler called before cache lookup.
//
//nolint:revive
func (h *NodeHandlers) EnrichModel(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*corev1.Node)
	m := resourceModel.(*model.Node)

	entityID := taggertypes.NewEntityID(taggertypes.KubernetesNode, r.Name)
	taggerTags, err := h.tagger.Tag(entityID, taggertypes.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for node %s: %s", r.Name, err)
		return
	}

	m.Tags = append(m.Tags, taggerTags...)
	return
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NodeHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.Node)
	m.Yaml = yaml
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *NodeHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.Node, 0, len(resourceModels))

	for _, r := range resourceModels {
		models = append(models, r.(*model.Node))
	}

	return &model.CollectorNode{
		ClusterName:  pctx.Cfg.KubeClusterName,
		ClusterId:    pctx.ClusterID,
		GroupId:      pctx.MsgGroupID,
		GroupSize:    int32(groupSize),
		Nodes:        models,
		Tags:         util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion: ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NodeHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*corev1.Node)
	return k8sTransformers.ExtractNode(ctx, r)
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NodeHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.Node)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NodeHandlers) ScrubBeforeMarshalling(ctx processors.ProcessorContext, resource interface{}) {
}

// GetNodeName is a handler called to retrieve the node name from the resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *NodeHandlers) GetNodeName(ctx processors.ProcessorContext, resource interface{}) string {
	r := resource.(*corev1.Node)
	return r.Name
}
