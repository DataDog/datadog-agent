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
	batchV1 "k8s.io/api/batch/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

var parralelism int32
var backoffLimit int32

func TestJobCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}
	parralelism = int32(2)
	backoffLimit = int32(5)

	jc := NewJobCollector(componentChannel, NewTestCommonClusterCollector(MockJobAPICollectorClient{}))
	expectedCollectorName := "Job Collector"
	RunCollectorTest(t, jc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Job 1",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:job:test-job-1",
				Type:       topology.Type{Name: "job"},
				Data: topology.Data{
					"name":               "test-job-1",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-job-1"),
					"backoffLimit":      &backoffLimit,
					"parallelism":       &parralelism,
				},
			},
		},
		{
			testCase: "Test Job 2",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:job:test-job-2",
				Type:       topology.Type{Name: "job"},
				Data: topology.Data{
					"name":               "test-job-2",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-job-2"),
					"backoffLimit":      &backoffLimit,
					"parallelism":       &parralelism,
				},
			},
		},
		{
			testCase: "Test Job 3 - Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:job:test-job-3",
				Type:       topology.Type{Name: "job"},
				Data: topology.Data{
					"name":               "test-job-3",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-job-3"),
					"kind":               "some-specified-kind",
					"generateName":       "some-specified-generation",
					"backoffLimit":      &backoffLimit,
					"parallelism":       &parralelism,
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			component := <-componentChannel
			assert.EqualValues(t, tc.expected, component)
		})
	}
}

type MockJobAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockJobAPICollectorClient) GetJobs() ([]batchV1.Job, error) {
	jobs := make([]batchV1.Job, 0)
	for i := 1; i <= 3; i++ {
		job := batchV1.Job{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-job-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-job-%d", i)),
				GenerateName: "",
			},
			Spec: batchV1.JobSpec{
				Parallelism: &parralelism,
				BackoffLimit: &backoffLimit,
			},
		}

		if i == 3 {
			job.TypeMeta.Kind = "some-specified-kind"
			job.ObjectMeta.GenerateName = "some-specified-generation"
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}
