// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNamespaceCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	nsc := NewNamespaceCollector(componentChannel, NewTestCommonClusterCollector(MockNamespaceAPICollectorClient{}))
	expectedCollectorName := "Namespace Collector"
	RunCollectorTest(t, nsc, expectedCollectorName)

	type test struct {
		testCase string
		expected *topology.Component
	}

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Namespace 1 - Complete",
			expected: &topology.Component{
				ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace-1",
				Type:       topology.Type{Name: "namespace"},
				Data: topology.Data{
					"name":              "test-namespace-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"uid":               types.UID("test-namespace-1"),
					"identifiers":       []string{"urn:kubernetes:/test-cluster-name:namespace/test-namespace-1"},
				},
			},
		},
		{
			testCase: "Test Namespace 2 - Minimal",
			expected: &topology.Component{
				ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace-2",
				Type:       topology.Type{Name: "namespace"},
				Data: topology.Data{
					"name":              "test-namespace-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"cluster-name": "test-cluster-name"},
					"uid":               types.UID("test-namespace-2"),
					"identifiers":       []string{"urn:kubernetes:/test-cluster-name:namespace/test-namespace-2"},
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

type MockNamespaceAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockNamespaceAPICollectorClient) GetNamespaces() ([]coreV1.Namespace, error) {
	namespaces := make([]coreV1.Namespace, 0)
	for i := 1; i <= 2; i++ {

		namespace := coreV1.Namespace{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-namespace-%d", i),
				CreationTimestamp: creationTime,
				UID:               types.UID(fmt.Sprintf("test-namespace-%d", i)),
				GenerateName:      "",
			},
		}

		if i != 2 {
			namespace.Labels = map[string]string{
				"test": "label",
			}
		}

		namespaces = append(namespaces, namespace)
	}

	return namespaces, nil
}
