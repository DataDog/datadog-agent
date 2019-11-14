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

func TestDeploymentCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}
	replicas = int32(1)

	cmc := NewDeploymentCollector(componentChannel, NewTestCommonClusterCollector(MockDeploymentAPICollectorClient{}))
	expectedCollectorName := "Deployment Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Deployment 1",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:deployment:test-namespace:test-deployment-1",
				Type:       topology.Type{Name: "deployment"},
				Data: topology.Data{
					"name":               "test-deployment-1",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-deployment-1"),
					"deploymentStrategy": appsV1.RollingUpdateDeploymentStrategyType,
					"desiredReplicas":    &replicas,
				},
			},
		},
		{
			testCase: "Test Deployment 2",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:deployment:test-namespace:test-deployment-2",
				Type:       topology.Type{Name: "deployment"},
				Data: topology.Data{
					"name":               "test-deployment-2",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-deployment-2"),
					"deploymentStrategy": appsV1.RollingUpdateDeploymentStrategyType,
					"desiredReplicas":    &replicas,
				},
			},
		},
		{
			testCase: "Test Deployment 3 - Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:deployment:test-namespace:test-deployment-3",
				Type:       topology.Type{Name: "deployment"},
				Data: topology.Data{
					"name":               "test-deployment-3",
					"creationTimestamp":  creationTime,
					"tags":               map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":          "test-namespace",
					"uid":                types.UID("test-deployment-3"),
					"kind":               "some-specified-kind",
					"generateName":       "some-specified-generation",
					"deploymentStrategy": appsV1.RollingUpdateDeploymentStrategyType,
					"desiredReplicas":    &replicas,
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

type MockDeploymentAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockDeploymentAPICollectorClient) GetDeployments() ([]appsV1.Deployment, error) {
	deployments := make([]appsV1.Deployment, 0)
	for i := 1; i <= 3; i++ {
		deployment := appsV1.Deployment{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-deployment-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-deployment-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.DeploymentSpec{
				Strategy: appsV1.DeploymentStrategy{
					Type: appsV1.RollingUpdateDeploymentStrategyType,
				},
				Replicas: &replicas,
			},
		}

		if i == 3 {
			deployment.TypeMeta.Kind = "some-specified-kind"
			deployment.ObjectMeta.GenerateName = "some-specified-generation"
		}

		deployments = append(deployments, deployment)
	}

	return deployments, nil
}
