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

	v2 "k8s.io/api/autoscaling/v2"
)

// HorizontalPodAutoscalerHandlers implements the Handlers interface for Kuberenetes HPAs.
type HorizontalPodAutoscalerHandlers = common.ModelHandlers[*v2.HorizontalPodAutoscaler, *model.HorizontalPodAutoscaler]

// NewHorizontalPodAutoscalerHandlers creates a new HorizontalPodAutoscalerHandlers.
func NewHorizontalPodAutoscalerHandlers(tagger tagger.Component) *HorizontalPodAutoscalerHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*v2.HorizontalPodAutoscaler, *model.HorizontalPodAutoscaler]{
		Extract: k8sTransformers.ExtractHorizontalPodAutoscaler,
		Tags:    func(m *model.HorizontalPodAutoscaler) *[]string { return &m.Tags },
		YAML:    func(m *model.HorizontalPodAutoscaler) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*v2.HorizontalPodAutoscaler],
		Message: func(h common.MessageHeader, models []*model.HorizontalPodAutoscaler) model.MessageBody {
			return &model.CollectorHorizontalPodAutoscaler{
				ClusterName:              h.ClusterName,
				ClusterId:                h.ClusterID,
				GroupId:                  h.GroupID,
				GroupSize:                h.GroupSize,
				HorizontalPodAutoscalers: models,
				Tags:                     h.Tags,
				AgentVersion:             h.AgentVersion,
			}
		},
	})
}
