// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"sort"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractCronJobV1(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	lastScheduleTime := metav1.NewTime(time.Date(2021, time.April, 17, 14, 30, 0, 0, time.UTC))
	lastSuccessfulTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	tests := map[string]struct {
		input             batchv1.CronJob
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		expected          model.CronJob
	}{
		"full cron job (active)": {
			input: batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Name:            "cronjob",
					Namespace:       "project",
					ResourceVersion: "220593670",
					UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
				},
				Spec: batchv1.CronJobSpec{
					ConcurrencyPolicy:          batchv1.ForbidConcurrent,
					FailedJobsHistoryLimit:     pointer.Ptr(int32(4)),
					Schedule:                   "*/5 * * * *",
					StartingDeadlineSeconds:    pointer.Ptr(int64(120)),
					SuccessfulJobsHistoryLimit: pointer.Ptr(int32(2)),
					Suspend:                    pointer.Ptr(false),
					TimeZone:                   pointer.Ptr("Europe/London"),
				},
				Status: batchv1.CronJobStatus{
					Active: []corev1.ObjectReference{
						{
							APIVersion:      "batch/v1",
							Kind:            "Job",
							Name:            "cronjob-1618585500",
							Namespace:       "project",
							ResourceVersion: "220593669",
							UID:             "644a62fe-783f-4609-bd2b-a9ec1212c07b",
						},
					},
					LastScheduleTime:   &lastScheduleTime,
					LastSuccessfulTime: &lastSuccessfulTime,
				},
			},
			labelsAsTags: map[string]string{
				"app": "application",
			},
			annotationsAsTags: map[string]string{
				"annotation": "annotation_key",
			},
			expected: model.CronJob{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"app:my-app"},
					Name:              "cronjob",
					Namespace:         "project",
					ResourceVersion:   "220593670",
					Uid:               "0ff96226-578d-4679-b3c8-72e8a485c0ef",
				},
				Spec: &model.CronJobSpec{
					ConcurrencyPolicy:          "Forbid",
					FailedJobsHistoryLimit:     4,
					Schedule:                   "*/5 * * * *",
					StartingDeadlineSeconds:    120,
					SuccessfulJobsHistoryLimit: 2,
					Suspend:                    false,
					TimeZone:                   "Europe/London",
				},
				Status: &model.CronJobStatus{
					Active: []*model.ObjectReference{
						{
							ApiVersion:      "batch/v1",
							Kind:            "Job",
							Name:            "cronjob-1618585500",
							Namespace:       "project",
							ResourceVersion: "220593669",
							Uid:             "644a62fe-783f-4609-bd2b-a9ec1212c07b",
						},
					},
					LastScheduleTime:   lastScheduleTime.Unix(),
					LastSuccessfulTime: lastSuccessfulTime.Unix(),
				},
				Tags: []string{
					"application:my-app",
					"annotation_key:my-annotation",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pctx := &processors.K8sProcessorContext{
				LabelsAsTags:      tc.labelsAsTags,
				AnnotationsAsTags: tc.annotationsAsTags,
			}
			actual := ExtractCronJobV1(pctx, &tc.input)
			sort.Strings(actual.Tags)
			sort.Strings(tc.expected.Tags)
			assert.Equal(t, &tc.expected, actual)
		})
	}
}
