// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_batch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	testhelpers "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/test"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestUpdateCronJobSchedule(t *testing.T) {
	const (
		namespace        = "production"
		cronJobName      = "data-refresh"
		originalSchedule = "0 * * * *"
	)

	tests := []struct {
		name              string
		schedule          *string
		timeZone          *string
		suspend           *bool
		dryRun            string
		initialSuspend    bool
		cronJobExists     bool
		wantErr           bool
		wantSchedule      string
		wantTimeZone      *string
		wantSuspend       bool
		wantPatchSchedule bool
		wantPatchTimeZone bool
		wantPatchSuspend  bool
	}{
		{
			name:              "updates schedule without changing suspended state",
			schedule:          stringPointer("*/15 * * * *"),
			initialSuspend:    true,
			cronJobExists:     true,
			wantSchedule:      "*/15 * * * *",
			wantSuspend:       true,
			wantPatchSchedule: true,
		},
		{
			name:              "updates schedule and suspends future runs",
			schedule:          stringPointer("0 2 * * *"),
			suspend:           boolPointer(true),
			cronJobExists:     true,
			wantSchedule:      "0 2 * * *",
			wantSuspend:       true,
			wantPatchSchedule: true,
			wantPatchSuspend:  true,
		},
		{
			name:              "updates time zone with dry run",
			timeZone:          stringPointer("America/New_York"),
			dryRun:            "All",
			cronJobExists:     true,
			wantSchedule:      originalSchedule,
			wantTimeZone:      stringPointer("America/New_York"),
			wantPatchTimeZone: true,
		},
		{
			name:             "resumes future runs without changing schedule",
			suspend:          boolPointer(false),
			initialSuspend:   true,
			cronJobExists:    true,
			wantSchedule:     originalSchedule,
			wantPatchSuspend: true,
		},
		{
			name:             "suspends future runs without changing schedule",
			suspend:          boolPointer(true),
			cronJobExists:    true,
			wantSchedule:     originalSchedule,
			wantSuspend:      true,
			wantPatchSuspend: true,
		},
		{
			name:          "rejects a request without an update field",
			cronJobExists: true,
			wantErr:       true,
		},
		{
			name:     "returns error when cron job does not exist",
			schedule: stringPointer("0 4 * * *"),
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			if test.cronJobExists {
				client = fake.NewSimpleClientset(&batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cronJobName,
						Namespace: namespace,
					},
					Spec: batchv1.CronJobSpec{
						Schedule: originalSchedule,
						Suspend:  boolPointer(test.initialSuspend),
					},
				})
			}

			inputs := map[string]interface{}{
				"namespace": namespace,
				"name":      cronJobName,
			}
			if test.schedule != nil {
				inputs["schedule"] = *test.schedule
			}
			if test.timeZone != nil {
				inputs["timeZone"] = *test.timeZone
			}
			if test.suspend != nil {
				inputs["suspend"] = *test.suspend
			}
			if test.dryRun != "" {
				inputs["dryRun"] = test.dryRun
			}

			handler := &UpdateCronJobScheduleHandler{client: client}
			outputs, err := handler.Run(context.Background(), newBatchTestTask(inputs), nil)
			if test.wantErr {
				require.Error(t, err)
				assert.Nil(t, outputs)
				return
			}

			require.NoError(t, err)
			output, ok := outputs.(*UpdateCronJobScheduleOutputs)
			require.True(t, ok)
			assert.Equal(t, cronJobName, output.CronJob.Name)
			assert.Equal(t, namespace, output.CronJob.Namespace)
			assert.Equal(t, test.wantSchedule, output.CronJob.Spec.Schedule)
			assert.Equal(t, test.wantTimeZone, output.CronJob.Spec.TimeZone)
			require.NotNil(t, output.CronJob.Spec.Suspend)
			assert.Equal(t, test.wantSuspend, *output.CronJob.Spec.Suspend)

			actions := client.Actions()
			require.Len(t, actions, 1)
			patchAction, ok := actions[0].(ktesting.PatchAction)
			require.True(t, ok)
			assert.Equal(t, "cronjobs", patchAction.GetResource().Resource)
			assert.Equal(t, namespace, patchAction.GetNamespace())
			assert.Equal(t, cronJobName, patchAction.GetName())
			assert.Equal(t, typesv1.MergePatchType, patchAction.GetPatchType())
			patchOptionsAction, ok := actions[0].(interface {
				GetPatchOptions() metav1.PatchOptions
			})
			require.True(t, ok)
			if test.dryRun == "" {
				assert.Nil(t, patchOptionsAction.GetPatchOptions().DryRun)
			} else {
				assert.Equal(t, []string{test.dryRun}, patchOptionsAction.GetPatchOptions().DryRun)
			}

			var patch struct {
				Spec struct {
					Schedule *string `json:"schedule,omitempty"`
					TimeZone *string `json:"timeZone,omitempty"`
					Suspend  *bool   `json:"suspend,omitempty"`
				} `json:"spec"`
			}
			require.NoError(t, json.Unmarshal(patchAction.GetPatch(), &patch))
			if test.wantPatchSchedule {
				require.NotNil(t, patch.Spec.Schedule)
				assert.Equal(t, test.wantSchedule, *patch.Spec.Schedule)
			} else {
				assert.Nil(t, patch.Spec.Schedule)
			}
			if test.wantPatchTimeZone {
				require.NotNil(t, patch.Spec.TimeZone)
				assert.Equal(t, *test.wantTimeZone, *patch.Spec.TimeZone)
			} else {
				assert.Nil(t, patch.Spec.TimeZone)
			}
			if test.wantPatchSuspend {
				require.NotNil(t, patch.Spec.Suspend)
				assert.Equal(t, test.wantSuspend, *patch.Spec.Suspend)
			} else {
				assert.Nil(t, patch.Spec.Suspend)
			}
		})
	}
}

func newBatchTestTask(inputs map[string]interface{}) *types.Task {
	return testhelpers.NewTestTask("task-id", "task-type", &types.Attributes{
		Name:     "Update cron job schedule",
		BundleID: "com.datadoghq.kubernetes.batch",
		Inputs:   inputs,
		OrgId:    123,
	})
}

func boolPointer(value bool) *bool {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
