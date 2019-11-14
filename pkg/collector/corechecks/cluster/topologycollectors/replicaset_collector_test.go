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
	appsV1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestReplicaSetCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}
	replicas = 1

	ic := NewReplicaSetCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockReplicaSetAPICollectorClient{}))
	expectedCollectorName := "ReplicaSet Collector"
	RunCollectorTest(t, ic, expectedCollectorName)

	for _, tc := range []struct {
		testCase          string
		expectedComponent *topology.Component
		expectedRelations []*topology.Relation
	}{
		{
			testCase: "Test ReplicaSet 1 - Minimal",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:replicaset:test-replicaset-1",
				Type:       topology.Type{Name: "replicaset"},
				Data: topology.Data{
					"name":              "test-replicaset-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-replicaset-1"),
					"desiredReplicas":   &replicas,
				},
			},
			expectedRelations: []*topology.Relation{},
		},
		{
			testCase: "Test ReplicaSet 2 - Kind + Generate Name",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:replicaset:test-replicaset-2",
				Type:       topology.Type{Name: "replicaset"},
				Data: topology.Data{
					"name":              "test-replicaset-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-replicaset-2"),
					"desiredReplicas":   &replicas,
					"kind":              "some-specified-kind",
					"generateName":      "some-specified-generation",
				},
			},
			expectedRelations: []*topology.Relation{},
		},
		{
			testCase: "Test ReplicaSet 3 - Complete",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:replicaset:test-replicaset-3",
				Type:       topology.Type{Name: "replicaset"},
				Data: topology.Data{
					"name":              "test-replicaset-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-replicaset-3"),
					"desiredReplicas":   &replicas,
					"kind":              "some-specified-kind",
					"generateName":      "some-specified-generation",
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:/kubernetes:test-cluster-name:deployment:test-namespace:test-deployment-3->urn:/kubernetes:test-cluster-name:replicaset:test-replicaset-3",
					Type:       topology.Type{Name: "controls"},
					SourceID:   "urn:/kubernetes:test-cluster-name:deployment:test-namespace:test-deployment-3",
					TargetID:   "urn:/kubernetes:test-cluster-name:replicaset:test-replicaset-3",
					Data:       map[string]interface{}{},
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			service := <-componentChannel
			assert.EqualValues(t, tc.expectedComponent, service)

			for _, expectedRelation := range tc.expectedRelations {
				serviceRelation := <-relationChannel
				assert.EqualValues(t, expectedRelation, serviceRelation)
			}
		})
	}
}

type MockReplicaSetAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockReplicaSetAPICollectorClient) GetReplicaSets() ([]appsV1.ReplicaSet, error) {
	replicaSets := make([]appsV1.ReplicaSet, 0)
	for i := 1; i <= 3; i++ {
		replicaSet := appsV1.ReplicaSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-replicaset-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-replicaset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.ReplicaSetSpec{
				Replicas: &replicas,
			},
		}

		if i > 1 {
			replicaSet.TypeMeta.Kind = "some-specified-kind"
			replicaSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		if i == 3 {
			replicaSet.OwnerReferences = []v1.OwnerReference{
				{Kind: "Deployment", Name: "test-deployment-3"},
			}
		}

		replicaSets = append(replicaSets, replicaSet)
	}

	return replicaSets, nil
}
