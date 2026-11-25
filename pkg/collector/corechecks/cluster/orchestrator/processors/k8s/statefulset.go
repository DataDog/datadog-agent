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
	"github.com/DataDog/datadog-agent/pkg/redact"

	appsv1 "k8s.io/api/apps/v1"
)

// StatefulSetHandlers implements the Handlers interface for Kubernetes StatefulSets.
type StatefulSetHandlers = common.ModelHandlers[*appsv1.StatefulSet, *model.StatefulSet]

// NewStatefulSetHandlers creates a new StatefulSetHandlers.
func NewStatefulSetHandlers(tagger tagger.Component) *StatefulSetHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*appsv1.StatefulSet, *model.StatefulSet]{
		Extract: k8sTransformers.ExtractStatefulSet,
		Tags:    func(m *model.StatefulSet) *[]string { return &m.Tags },
		YAML:    func(m *model.StatefulSet) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*appsv1.StatefulSet],
		Scrub:   func(r *appsv1.StatefulSet, s *redact.DataScrubber) { redact.ScrubPodTemplateSpec(&r.Spec.Template, s) },
		Message: func(h common.MessageHeader, models []*model.StatefulSet) model.MessageBody {
			return &model.CollectorStatefulSet{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				StatefulSets: models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
