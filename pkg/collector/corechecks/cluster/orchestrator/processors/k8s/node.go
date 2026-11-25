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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"

	corev1 "k8s.io/api/core/v1"
)

// NodeHandlers implements the Handlers interface for Kubernetes Nodes. It adds the node name of the manifest to the
// generic model handlers.
type NodeHandlers struct {
	common.ModelHandlers[*corev1.Node, *model.Node]
}

// NewNodeHandlers creates a new NodeHandlers.
func NewNodeHandlers(tagger tagger.Component) *NodeHandlers {
	return &NodeHandlers{ModelHandlers: *common.NewModelHandlers(tagger, common.ModelSpec[*corev1.Node, *model.Node]{
		Extract: k8sTransformers.ExtractNode,
		Tags:    func(m *model.Node) *[]string { return &m.Tags },
		YAML:    func(m *model.Node) *[]byte { return &m.Yaml },
		Entity: func(_ processors.ProcessorContext, r *corev1.Node) taggertypes.EntityID {
			return taggertypes.NewEntityID(taggertypes.KubernetesNode, r.Name)
		},
		Message: func(h common.MessageHeader, models []*model.Node) model.MessageBody {
			return &model.CollectorNode{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Nodes:        models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})}
}

// GetNodeName returns the name of the node, which its manifest reports as its node name.
func (h *NodeHandlers) GetNodeName(_ processors.ProcessorContext, resource interface{}) string {
	return resource.(*corev1.Node).Name
}
