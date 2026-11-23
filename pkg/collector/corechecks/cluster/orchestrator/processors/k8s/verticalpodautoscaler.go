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

	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

// VerticalPodAutoscalerHandlers implements the Handlers interface for Kuberenetes VPAs.
type VerticalPodAutoscalerHandlers = common.ModelHandlers[*v1.VerticalPodAutoscaler, *model.VerticalPodAutoscaler]

// NewVerticalPodAutoscalerHandlers creates a new VerticalPodAutoscalerHandlers.
func NewVerticalPodAutoscalerHandlers(tagger tagger.Component) *VerticalPodAutoscalerHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*v1.VerticalPodAutoscaler, *model.VerticalPodAutoscaler]{
		Extract: k8sTransformers.ExtractVerticalPodAutoscaler,
		Tags:    func(m *model.VerticalPodAutoscaler) *[]string { return &m.Tags },
		YAML:    func(m *model.VerticalPodAutoscaler) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*v1.VerticalPodAutoscaler],
		Message: func(h common.MessageHeader, models []*model.VerticalPodAutoscaler) model.MessageBody {
			return &model.CollectorVerticalPodAutoscaler{
				ClusterName:            h.ClusterName,
				ClusterId:              h.ClusterID,
				GroupId:                h.GroupID,
				GroupSize:              h.GroupSize,
				VerticalPodAutoscalers: models,
				Tags:                   h.Tags,
				AgentVersion:           h.AgentVersion,
			}
		},
	})
}
