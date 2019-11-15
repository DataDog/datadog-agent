// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/batch/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestCronJobCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cjc := NewCronJobCollector(componentChannel, NewTestCommonClusterCollector(MockCronJobAPICollectorClient{}))
	expectedCollectorName := "CronJob Collector"
	RunCollectorTest(t, cjc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Cron Job 1 - Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1",
				Type:       topology.Type{Name: "cronjob"},
				Data: topology.Data{
					"schedule":          "0 0 * * *",
					"name":              "test-cronjob-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-cronjob-1"),
					"concurrencyPolicy": v1beta1.AllowConcurrent,
					"kind":              "some-specified-kind",
					"generateName":      "some-specified-generation",
				},
			},
		},
		{
			testCase: "Test Cron Job 2 - Minimal",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-2",
				Type:       topology.Type{Name: "cronjob"},
				Data: topology.Data{
					"schedule":          "0 0 * * *",
					"name":              "test-cronjob-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-cronjob-2"),
					"concurrencyPolicy": v1beta1.AllowConcurrent,
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			cronJob := <-componentChannel
			assert.EqualValues(t, tc.expected, cronJob)
		})
	}
}

type MockCronJobAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockCronJobAPICollectorClient) GetCronJobs() ([]v1beta1.CronJob, error) {
	cronJobs := make([]v1beta1.CronJob, 0)
	for i := 1; i <= 2; i++ {
		job := v1beta1.CronJob{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-cronjob-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-cronjob-%d", i)),
				GenerateName: "",
			},
			Spec: v1beta1.CronJobSpec{
				Schedule:          "0 0 * * *",
				ConcurrencyPolicy: v1beta1.AllowConcurrent,
			},
		}

		if i == 1 {
			job.TypeMeta.Kind = "some-specified-kind"
			job.ObjectMeta.GenerateName = "some-specified-generation"
		}

		cronJobs = append(cronJobs, job)
	}

	return cronJobs, nil
}
