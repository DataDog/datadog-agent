// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"

	corev1 "k8s.io/api/core/v1"
)

// NamespaceHandlers implements the Handlers interface for Kubernetes Namespace.
type NamespaceHandlers = common.ModelHandlers[*corev1.Namespace, *model.Namespace]

// NewNamespaceHandlers creates a new NamespaceHandlers.
func NewNamespaceHandlers(tagger tagger.Component) *NamespaceHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*corev1.Namespace, *model.Namespace]{
		Extract: k8sTransformers.ExtractNamespace,
		Tags:    func(m *model.Namespace) *[]string { return &m.Tags },
		YAML:    func(m *model.Namespace) *[]byte { return &m.Yaml },
		Entity:  common.ClusterMetadataEntity[*corev1.Namespace],
		Message: func(h common.MessageHeader, models []*model.Namespace) model.MessageBody {
			return &model.CollectorNamespace{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Namespaces:   models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
