// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestCronJobV1Collector(t *testing.T) {
	creationTime := CreateTestTime()
	lastScheduleTime := metav1.NewTime(creationTime.Add(24 * time.Hour))
	lastSuccessfulTime := metav1.NewTime(creationTime.Time)

	cronJob := &batchv1.CronJob{
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
			ResourceVersion: "1206",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
		},
		Spec: batchv1.CronJobSpec{
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			FailedJobsHistoryLimit:     pointer.Ptr(int32(4)),
			Schedule:                   "*/5 * * * *",
			StartingDeadlineSeconds:    pointer.Ptr(int64(120)),
			SuccessfulJobsHistoryLimit: pointer.Ptr(int32(2)),
			Suspend:                    pointer.Ptr(false),
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
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewCronJobV1Collector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{cronJob},
		ExpectedMetadataType:       &model.CollectorCronJob{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
