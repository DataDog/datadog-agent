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

// ServiceHandlers implements the Handlers interface for Kubernetes Services.
type ServiceHandlers = common.ModelHandlers[*corev1.Service, *model.Service]

// NewServiceHandlers creates a new ServiceHandlers.
func NewServiceHandlers(tagger tagger.Component) *ServiceHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*corev1.Service, *model.Service]{
		Extract: k8sTransformers.ExtractService,
		Tags:    func(m *model.Service) *[]string { return &m.Tags },
		YAML:    func(m *model.Service) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*corev1.Service],
		Message: func(h common.MessageHeader, models []*model.Service) model.MessageBody {
			return &model.CollectorService{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Services:     models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
