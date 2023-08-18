// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	batchv1 "k8s.io/api/batch/v1"
)

// ExtractCronJobV1 returns the protobuf model corresponding to a Kubernetes
// CronJob resource.
func ExtractCronJobV1(cj *batchv1.CronJob) *model.CronJob {
	cronJob := model.CronJob{
		Metadata: extractMetadata(&cj.ObjectMeta),
		Spec: &model.CronJobSpec{
			ConcurrencyPolicy: string(cj.Spec.ConcurrencyPolicy),
			Schedule:          cj.Spec.Schedule,
		},
		Status: &model.CronJobStatus{},
	}

	if cj.Spec.FailedJobsHistoryLimit != nil {
		cronJob.Spec.FailedJobsHistoryLimit = *cj.Spec.FailedJobsHistoryLimit
	}
	if cj.Spec.StartingDeadlineSeconds != nil {
		cronJob.Spec.StartingDeadlineSeconds = *cj.Spec.StartingDeadlineSeconds
	}
	if cj.Spec.SuccessfulJobsHistoryLimit != nil {
		cronJob.Spec.SuccessfulJobsHistoryLimit = *cj.Spec.SuccessfulJobsHistoryLimit
	}
	if cj.Spec.Suspend != nil {
		cronJob.Spec.Suspend = *cj.Spec.Suspend
	}

	if cj.Status.LastScheduleTime != nil {
		cronJob.Status.LastScheduleTime = cj.Status.LastScheduleTime.Unix()
	}
	for _, job := range cj.Status.Active {
		cronJob.Status.Active = append(cronJob.Status.Active, &model.ObjectReference{
			ApiVersion:      job.APIVersion,
			FieldPath:       job.FieldPath,
			Kind:            job.Kind,
			Name:            job.Name,
			Namespace:       job.Namespace,
			ResourceVersion: job.ResourceVersion,
			Uid:             string(job.UID),
		})
	}

	cronJob.Spec.ResourceRequirements = ExtractPodTemplateResourceRequirements(cj.Spec.JobTemplate.Spec.Template)
	cronJob.Tags = append(cronJob.Tags, transformers.RetrieveUnifiedServiceTags(cj.ObjectMeta.Labels)...)

	return &cronJob
}
