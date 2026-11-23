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

	netv1 "k8s.io/api/networking/v1"
)

// IngressHandlers implements the Handlers interface for Kubernetes Ingresss.
type IngressHandlers = common.ModelHandlers[*netv1.Ingress, *model.Ingress]

// NewIngressHandlers creates a new IngressHandlers.
func NewIngressHandlers(tagger tagger.Component) *IngressHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*netv1.Ingress, *model.Ingress]{
		Extract: k8sTransformers.ExtractIngress,
		Tags:    func(m *model.Ingress) *[]string { return &m.Tags },
		YAML:    func(m *model.Ingress) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*netv1.Ingress],
		Message: func(h common.MessageHeader, models []*model.Ingress) model.MessageBody {
			return &model.CollectorIngress{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Ingresses:    models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
