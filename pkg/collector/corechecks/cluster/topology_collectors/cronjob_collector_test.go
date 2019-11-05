// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topology_collectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/batch/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestCronJobCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)

	creationTime = v1.Time{ Time: time.Now().Add(-1*time.Hour) }

	cjc := NewCronJobCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockCronJobAPICollectorClient{}))
	expectedCollectorName := "CronJob Collector"
	RunCollectorTest(t, cjc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expectedComponent *topology.Component
		expectedRelations []*topology.Relation
	}{
		{
			testCase: "Test Cron Job 1 - CronJob + Job Relations",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1",
				Type:       topology.Type{Name: "cronjob"},
				Data: topology.Data{
					"schedule": "0 0 * * *",
					"name": "test-cronjob-1",
					"creationTimestamp": creationTime,
					"tags": map[string]string{"test":"label", "cluster-name":"test-cluster-name"},
					"namespace": "test-namespace",
					"uid": types.UID("test-cronjob-1"),
					"concurrencyPolicy": v1beta1.AllowConcurrent,
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1->urn:/kubernetes:test-cluster-name:job:job-1",
					Type:       topology.Type{Name: "creates"},
					SourceID:   "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1",
					TargetID:   "urn:/kubernetes:test-cluster-name:job:job-1",
					Data: map[string]interface {}{},
				},
				{
					ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1->urn:/kubernetes:test-cluster-name:job:job-2",
					Type:       topology.Type{Name: "creates"},
					SourceID:   "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-1",
					TargetID:   "urn:/kubernetes:test-cluster-name:job:job-2",
					Data: map[string]interface {}{},
				},
			},
		},
		{
			testCase: "Test Cron Job 2 - Minimal",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:cronjob:test-cronjob-2",
				Type:       topology.Type{Name: "cronjob"},
				Data: topology.Data{
					"schedule": "0 0 * * *",
					"name": "test-cronjob-2",
					"creationTimestamp": creationTime,
					"tags": map[string]string{"test":"label", "cluster-name":"test-cluster-name"},
					"namespace": "test-namespace",
					"uid": types.UID("test-cronjob-2"),
					"concurrencyPolicy": v1beta1.AllowConcurrent,
				},
			},
			expectedRelations: []*topology.Relation{},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			cronJob := <- componentChannel
			assert.EqualValues(t, tc.expectedComponent, cronJob)

			for _, expectedRelation := range tc.expectedRelations {
				jobRelation := <- relationChannel
				assert.EqualValues(t, expectedRelation, jobRelation)
			}
		})
	}
}

type MockCronJobAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockCronJobAPICollectorClient) GetCronJobs() ([]v1beta1.CronJob, error) {
	cronJobs := make([]v1beta1.CronJob, 0)
	for i := 1; i <= 2; i++ {

		var jobLinks []coreV1.ObjectReference
		if i == 1 {
			jobLinks = []coreV1.ObjectReference{
				{ Name: "job-1", Namespace: "test-namespace-1" },
				{ Name: "job-2", Namespace: "test-namespace-2" },
			}
		} else {
			jobLinks = []coreV1.ObjectReference{}
		}

		cronJobs = append(cronJobs, v1beta1.CronJob{
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
			Spec: v1beta1.CronJobSpec {
				Schedule: "0 0 * * *",
				ConcurrencyPolicy: v1beta1.AllowConcurrent,
			},
			Status: v1beta1.CronJobStatus{
				Active: jobLinks,
			},
		})
	}

	return cronJobs, nil
}
