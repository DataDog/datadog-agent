// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
)

func TestScrubJob(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: sensitiveObjectMeta(), Spec: batchv1.JobSpec{Template: sensitivePodTemplateSpec()}}
	ScrubJob(job, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, job.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, job.Spec.Template)
}

func TestScrubCronJob(t *testing.T) {
	cronJob := &batchv1.CronJob{
		ObjectMeta: sensitiveObjectMeta(),
		Spec: batchv1.CronJobSpec{JobTemplate: batchv1.JobTemplateSpec{
			ObjectMeta: sensitiveObjectMeta(),
			Spec:       batchv1.JobSpec{Template: sensitivePodTemplateSpec()},
		}},
	}
	ScrubCronJob(cronJob, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, cronJob.ObjectMeta)
	assertScrubbedObjectMeta(t, cronJob.Spec.JobTemplate.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, cronJob.Spec.JobTemplate.Spec.Template)
}
