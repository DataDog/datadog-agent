// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	batchv1 "k8s.io/api/batch/v1"
)

// ScrubJob scrubs a job.
func ScrubJob(j *batchv1.Job, scrubber *DataScrubber) {
	ScrubObjectMeta(&j.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&j.Spec.Template, scrubber)
}

// ScrubCronJob scrubs a cron job.
func ScrubCronJob(c *batchv1.CronJob, scrubber *DataScrubber) {
	ScrubObjectMeta(&c.ObjectMeta, scrubber)
	ScrubObjectMeta(&c.Spec.JobTemplate.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&c.Spec.JobTemplate.Spec.Template, scrubber)
}
