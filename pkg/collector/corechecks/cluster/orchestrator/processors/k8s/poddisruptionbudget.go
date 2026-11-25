// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"

	policyv1 "k8s.io/api/policy/v1"
)

// PodDisruptionBudgetHandlers implements the Handlers interface for Kubernetes NetworkPolicy.
type PodDisruptionBudgetHandlers = common.ModelHandlers[*policyv1.PodDisruptionBudget, *model.PodDisruptionBudget]

// NewPodDisruptionBudgetHandlers creates a new PodDisruptionBudgetHandlers.
func NewPodDisruptionBudgetHandlers(tagger tagger.Component) *PodDisruptionBudgetHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*policyv1.PodDisruptionBudget, *model.PodDisruptionBudget]{
		Extract: k8sTransformers.ExtractPodDisruptionBudget,
		Tags:    func(m *model.PodDisruptionBudget) *[]string { return &m.Tags },
		Entity:  common.MetadataEntity[*policyv1.PodDisruptionBudget],
		Message: func(h common.MessageHeader, models []*model.PodDisruptionBudget) model.MessageBody {
			return &model.CollectorPodDisruptionBudget{
				ClusterName:          h.ClusterName,
				ClusterId:            h.ClusterID,
				GroupId:              h.GroupID,
				GroupSize:            h.GroupSize,
				PodDisruptionBudgets: models,
				Tags:                 h.Tags,
				AgentVersion:         h.AgentVersion,
			}
		},
	})
}
