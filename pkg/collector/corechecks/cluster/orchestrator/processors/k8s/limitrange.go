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

// LimitRangeHandlers implements the Handlers interface for Kubernetes LimitRange.
type LimitRangeHandlers = common.ModelHandlers[*corev1.LimitRange, *model.LimitRange]

// NewLimitRangeHandlers creates a new LimitRangeHandlers.
func NewLimitRangeHandlers(tagger tagger.Component) *LimitRangeHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*corev1.LimitRange, *model.LimitRange]{
		Extract: k8sTransformers.ExtractLimitRange,
		Tags:    func(m *model.LimitRange) *[]string { return &m.Tags },
		Entity:  common.MetadataEntity[*corev1.LimitRange],
		Message: func(h common.MessageHeader, models []*model.LimitRange) model.MessageBody {
			return &model.CollectorLimitRange{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				LimitRanges:  models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
