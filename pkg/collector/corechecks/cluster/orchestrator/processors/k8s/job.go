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

	batchv1 "k8s.io/api/batch/v1"
)

// JobHandlers implements the Handlers interface for Kubernetes Jobs.
type JobHandlers = common.ModelHandlers[*batchv1.Job, *model.Job]

// NewJobHandlers creates a new JobHandlers.
func NewJobHandlers(tagger tagger.Component) *JobHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*batchv1.Job, *model.Job]{
		Extract: k8sTransformers.ExtractJob,
		Tags:    func(m *model.Job) *[]string { return &m.Tags },
		YAML:    func(m *model.Job) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*batchv1.Job],
		Scrub:   func(r *batchv1.Job, s *redact.DataScrubber) { redact.ScrubPodTemplateSpec(&r.Spec.Template, s) },
		Message: func(h common.MessageHeader, models []*model.Job) model.MessageBody {
			return &model.CollectorJob{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Jobs:         models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
