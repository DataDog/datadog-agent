// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

func TestExtendedJobFactoryConditionStartTimeMetrics(t *testing.T) {
	created := time.Unix(1000, 0)
	started := time.Unix(1060, 0)

	newJob := func(startTime *metav1.Time, conditions ...batchv1.JobCondition) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "bar-29000001",
				Namespace:         "foo",
				CreationTimestamp: metav1.NewTime(created),
			},
			Status: batchv1.JobStatus{
				StartTime:  startTime,
				Conditions: conditions,
			},
		}
	}
	condition := func(conditionType batchv1.JobConditionType, status corev1.ConditionStatus) batchv1.JobCondition {
		return batchv1.JobCondition{Type: conditionType, Status: status}
	}
	startTime := metav1.NewTime(started)

	tests := []struct {
		name             string
		job              *batchv1.Job
		expectedComplete []float64
		expectedFailed   []float64
	}{
		{
			name:             "complete job reports its start time",
			job:              newJob(&startTime, condition(batchv1.JobComplete, corev1.ConditionTrue)),
			expectedComplete: []float64{1060},
		},
		{
			name: "failed job reports its start time",
			job: newJob(&startTime,
				condition(batchv1.JobFailureTarget, corev1.ConditionTrue),
				condition(batchv1.JobFailed, corev1.ConditionTrue),
			),
			expectedFailed: []float64{1060},
		},
		{
			name:           "job without start time falls back to creation time",
			job:            newJob(nil, condition(batchv1.JobFailed, corev1.ConditionTrue)),
			expectedFailed: []float64{1000},
		},
		{
			name: "job with a false condition reports nothing",
			job:  newJob(&startTime, condition(batchv1.JobComplete, corev1.ConditionFalse)),
		},
		{
			name: "running job reports nothing",
			job:  newJob(&startTime),
		},
	}

	generators := (&extendedJobFactory{}).MetricFamilyGenerators()
	findGenerator := func(name string) generator.FamilyGenerator {
		for _, g := range generators {
			if g.Name == name {
				return g
			}
		}
		require.FailNow(t, "generator not found", name)
		return generator.FamilyGenerator{}
	}
	completeGenerator := findGenerator("kube_job_complete_start_time")
	failedGenerator := findGenerator("kube_job_failed_start_time")

	values := func(g generator.FamilyGenerator, job *batchv1.Job) []float64 {
		var vals []float64
		for _, m := range g.Generate(job).Metrics {
			assert.Equal(t, []string{"namespace", "job_name"}, m.LabelKeys)
			assert.Equal(t, []string{"foo", "bar-29000001"}, m.LabelValues)
			vals = append(vals, m.Value)
		}
		return vals
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedComplete, values(completeGenerator, tt.job))
			assert.Equal(t, tt.expectedFailed, values(failedGenerator, tt.job))
		})
	}
}
