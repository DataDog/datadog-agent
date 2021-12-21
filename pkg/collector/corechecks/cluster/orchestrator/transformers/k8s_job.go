// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package transformers

import (
	model "github.com/DataDog/agent-payload/v5/process"
	batchv1 "k8s.io/api/batch/v1"
)

// ExtractK8sJob returns the protobuf model corresponding to a Kubernetes Job
// resource.
func ExtractK8sJob(j *batchv1.Job) *model.Job {
	job := model.Job{
		Metadata: extractMetadata(&j.ObjectMeta),
		Spec:     &model.JobSpec{},
		Status: &model.JobStatus{
			Active:           j.Status.Active,
			ConditionMessage: extractJobConditionMessage(j.Status.Conditions),
			Failed:           j.Status.Failed,
			Succeeded:        j.Status.Succeeded,
		},
	}

	if j.Spec.ActiveDeadlineSeconds != nil {
		job.Spec.ActiveDeadlineSeconds = *j.Spec.ActiveDeadlineSeconds
	}
	if j.Spec.BackoffLimit != nil {
		job.Spec.BackoffLimit = *j.Spec.BackoffLimit
	}
	if j.Spec.Completions != nil {
		job.Spec.Completions = *j.Spec.Completions
	}
	if j.Spec.ManualSelector != nil {
		job.Spec.ManualSelector = *j.Spec.ManualSelector
	}
	if j.Spec.Parallelism != nil {
		job.Spec.Parallelism = *j.Spec.Parallelism
	}
	if j.Spec.Selector != nil {
		job.Spec.Selectors = extractLabelSelector(j.Spec.Selector)
	}

	if j.Status.StartTime != nil {
		job.Status.StartTime = j.Status.StartTime.Unix()
	}
	if j.Status.CompletionTime != nil {
		job.Status.CompletionTime = j.Status.CompletionTime.Unix()
	}

	return &job
}

func extractJobConditionMessage(conditions []batchv1.JobCondition) string {
	for _, c := range conditions {
		if c.Type == batchv1.JobFailed && c.Message != "" {
			return c.Message
		}
	}
	return ""
}
