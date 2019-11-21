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
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestPodCollector(t *testing.T) {
	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)
	containerCorrelationChannel := make(chan *ContainerCorrelation)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	ic := NewPodCollector(componentChannel, relationChannel, containerCorrelationChannel, NewTestCommonClusterCollector(MockPodAPICollectorClient{}))
	expectedCollectorName := "Pod Collector"
	RunCollectorTest(t, ic, expectedCollectorName)

	for _, tc := range []struct {
		testCase   string
		assertions []func()
	}{
		{
			testCase: "Test Pod 1 - Minimal",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-1",
						Type:       topology.Type{Name: "pod"},
						Data: topology.Data{
							"name":              "test-pod-1",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-pod-1"),
							"identifiers":       []string{"urn:ip:/test-cluster-name:test-pod-1:10.0.0.1"},
							"restartPolicy":     coreV1.RestartPolicyAlways,
							"status": coreV1.PodStatus{
								Phase:                 coreV1.PodRunning,
								Conditions:            []coreV1.PodCondition{},
								InitContainerStatuses: []coreV1.ContainerStatus{},
								ContainerStatuses:     []coreV1.ContainerStatus{},
								StartTime:             &creationTime,
								PodIP:                 "10.0.0.1",
							},
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-1->urn:/kubernetes:test-cluster-name:node:test-node",
						Type:       topology.Type{Name: "scheduled_on"},
						SourceID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-1",
						TargetID:   "urn:/kubernetes:test-cluster-name:node:test-node",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
			},
		},
		{
			testCase: "Test Pod 2 - All Metadata",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-2",
						Type:       topology.Type{Name: "pod"},
						Data: topology.Data{
							"name":              "test-pod-2",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "service-account": "some-service-account-name", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-pod-2"),
							"identifiers":       []string{"urn:ip:/test-cluster-name:10.0.0.2", "urn:ip:/test-cluster-name:test-pod-2:10.0.0.2"},
							"restartPolicy":     coreV1.RestartPolicyAlways,
							"kind":              "some-specified-kind",
							"generateName":      "some-specified-generation",
							"status": coreV1.PodStatus{
								Phase:                 coreV1.PodRunning,
								Conditions:            []coreV1.PodCondition{},
								InitContainerStatuses: []coreV1.ContainerStatus{},
								ContainerStatuses:     []coreV1.ContainerStatus{},
								StartTime:             &creationTime,
								PodIP:                 "10.0.0.2",
								Message:               "some longer readable message for the phase",
								Reason:                "some-short-reason",
								NominatedNodeName:     "some-nominated-node-name",
								QOSClass:              "some-qos-class",
							},
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-2->urn:/kubernetes:test-cluster-name:node:test-node",
						Type:       topology.Type{Name: "scheduled_on"},
						SourceID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-2",
						TargetID:   "urn:/kubernetes:test-cluster-name:node:test-node",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
			},
		},
		{
			testCase: "Test Pod 3 - All Controllers: Daemonset, Deployment, Job, ReplicaSet, StatefulSet",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "pod"},
						Data: topology.Data{
							"name":              "test-pod-3",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-pod-3"),
							"identifiers":       []string{"urn:ip:/test-cluster-name:test-pod-3:10.0.0.1"},
							"restartPolicy":     coreV1.RestartPolicyAlways,
							"status": coreV1.PodStatus{
								Phase:                 coreV1.PodRunning,
								Conditions:            []coreV1.PodCondition{},
								InitContainerStatuses: []coreV1.ContainerStatus{},
								ContainerStatuses:     []coreV1.ContainerStatus{},
								StartTime:             &creationTime,
								PodIP:                 "10.0.0.1",
							},
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:pod:test-pod-3->urn:/kubernetes:test-cluster-name:node:test-node",
						Type:       topology.Type{Name: "scheduled_on"},
						SourceID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						TargetID:   "urn:/kubernetes:test-cluster-name:node:test-node",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:daemonset:daemonset-v->urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "controls"},
						SourceID:   "urn:/kubernetes:test-cluster-name:daemonset:daemonset-v",
						TargetID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:deployment:test-namespace:deployment-w->urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "controls"},
						SourceID:   "urn:/kubernetes:test-cluster-name:deployment:test-namespace:deployment-w",
						TargetID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:job:job-x->urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "controls"},
						SourceID:   "urn:/kubernetes:test-cluster-name:job:job-x",
						TargetID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:replicaset:replicaset-y->urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "controls"},
						SourceID:   "urn:/kubernetes:test-cluster-name:replicaset:replicaset-y",
						TargetID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:statefulset:statefulset-z->urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Type:       topology.Type{Name: "controls"},
						SourceID:   "urn:/kubernetes:test-cluster-name:statefulset:statefulset-z",
						TargetID:   "urn:/kubernetes:test-cluster-name:pod:test-pod-3",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
			},
		},
		{
			testCase: "Test Pod 4 - Volumes + Persistent Volumes",
		},
		{
			testCase: "Test Pod 5 - Containers + Container Correlation",
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			for _, assertion := range tc.assertions {
				assertion()
			}
		})
	}
}

type MockPodAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockPodAPICollectorClient) GetPods() ([]coreV1.Pod, error) {
	pods := make([]coreV1.Pod, 0)
	for i := 1; i <= 3; i++ {
		pod := coreV1.Pod{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-pod-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-pod-%d", i)),
				GenerateName: "",
			},
			Status: coreV1.PodStatus{
				Phase:     coreV1.PodRunning,
				PodIP:     "10.0.0.1",
				StartTime: &creationTime,
			},
			Spec: coreV1.PodSpec{
				RestartPolicy: coreV1.RestartPolicyAlways,
				NodeName:      "test-node",
			},
		}

		if i == 2 {
			pod.Spec.HostNetwork = true
			pod.Status.PodIP = "10.0.0.2"
			pod.Spec.ServiceAccountName = "some-service-account-name"
			pod.Status.Message = "some longer readable message for the phase"
			pod.Status.Reason = "some-short-reason"
			pod.Status.NominatedNodeName = "some-nominated-node-name"
			pod.Status.QOSClass = "some-qos-class"
			pod.TypeMeta.Kind = "some-specified-kind"
			pod.ObjectMeta.GenerateName = "some-specified-generation"
		}

		if i == 3 {
			pod.OwnerReferences = []v1.OwnerReference{
				{Kind: "DaemonSet", Name: "daemonset-v"},
				{Kind: "Deployment", Name: "deployment-w"},
				{Kind: "Job", Name: "job-x"},
				{Kind: "ReplicaSet", Name: "replicaset-y"},
				{Kind: "StatefulSet", Name: "statefulset-z"},
			}
		}

		if i == 4 {

		}

		if i == 5 {

		}

		pods = append(pods, pod)
	}

	return pods, nil
}
