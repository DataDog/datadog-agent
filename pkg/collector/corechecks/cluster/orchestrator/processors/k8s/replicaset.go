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

// ReplicaSetHandlers implements the Handlers interface for Kubernetes ReplicaSets.
type ReplicaSetHandlers = common.ModelHandlers[*appsv1.ReplicaSet, *model.ReplicaSet]

// NewReplicaSetHandlers creates a new ReplicaSetHandlers.
func NewReplicaSetHandlers(tagger tagger.Component) *ReplicaSetHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*appsv1.ReplicaSet, *model.ReplicaSet]{
		Extract: k8sTransformers.ExtractReplicaSet,
		Tags:    func(m *model.ReplicaSet) *[]string { return &m.Tags },
		YAML:    func(m *model.ReplicaSet) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*appsv1.ReplicaSet],
		Scrub:   func(r *appsv1.ReplicaSet, s *redact.DataScrubber) { redact.ScrubPodTemplateSpec(&r.Spec.Template, s) },
		Message: func(h common.MessageHeader, models []*model.ReplicaSet) model.MessageBody {
			return &model.CollectorReplicaSet{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				ReplicaSets:  models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
