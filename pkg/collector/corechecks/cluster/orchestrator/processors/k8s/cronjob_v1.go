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

// CronJobV1Handlers implements the Handlers interface for Kubernetes CronJobs.
type CronJobV1Handlers = common.ModelHandlers[*batchv1.CronJob, *model.CronJob]

// NewCronJobV1Handlers creates a new CronJobV1Handlers.
func NewCronJobV1Handlers(tagger tagger.Component) *CronJobV1Handlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*batchv1.CronJob, *model.CronJob]{
		Extract: k8sTransformers.ExtractCronJobV1,
		Tags:    func(m *model.CronJob) *[]string { return &m.Tags },
		YAML:    func(m *model.CronJob) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*batchv1.CronJob],
		Scrub: func(r *batchv1.CronJob, s *redact.DataScrubber) {
			redact.ScrubPodTemplateSpec(&r.Spec.JobTemplate.Spec.Template, s)
		},
		Message: func(h common.MessageHeader, models []*model.CronJob) model.MessageBody {
			return &model.CollectorCronJob{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				CronJobs:     models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
