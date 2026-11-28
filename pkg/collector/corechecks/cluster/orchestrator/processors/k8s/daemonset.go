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

// DaemonSetHandlers implements the Handlers interface for Kubernetes DaemonSets.
type DaemonSetHandlers = common.ModelHandlers[*appsv1.DaemonSet, *model.DaemonSet]

// NewDaemonSetHandlers creates a new DaemonSetHandlers.
func NewDaemonSetHandlers(tagger tagger.Component) *DaemonSetHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*appsv1.DaemonSet, *model.DaemonSet]{
		Extract: k8sTransformers.ExtractDaemonSet,
		Tags:    func(m *model.DaemonSet) *[]string { return &m.Tags },
		YAML:    func(m *model.DaemonSet) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*appsv1.DaemonSet],
		Scrub:   func(r *appsv1.DaemonSet, s *redact.DataScrubber) { redact.ScrubPodTemplateSpec(&r.Spec.Template, s) },
		Message: func(h common.MessageHeader, models []*model.DaemonSet) model.MessageBody {
			return &model.CollectorDaemonSet{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				DaemonSets:   models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
