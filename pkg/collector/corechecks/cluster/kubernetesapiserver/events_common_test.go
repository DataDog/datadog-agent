// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/local"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func TestGetDDAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    event.AlertType
	}{
		{
			name:    "normal",
			k8sType: "Normal",
			want:    event.AlertTypeInfo,
		},
		{
			name:    "warning",
			k8sType: "Warning",
			want:    event.AlertTypeWarning,
		},
		{
			name:    "unknown",
			k8sType: "Unknown",
			want:    event.AlertTypeInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDDAlertType(tt.k8sType)
			assert.Equal(t, got, tt.want)
		})
	}
}

func Test_getInvolvedObjectTags(t *testing.T) {
	taggerInstance := local.NewFakeTagger()
	taggerInstance.SetTags("kubernetes_metadata://namespaces//default", "workloadmeta-kubernetes_node", []string{"team:container-int"}, nil, nil, nil)
	tests := []struct {
		name           string
		involvedObject v1.ObjectReference
		tags           []string
	}{
		{
			name: "get pod basic tags",
			involvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "my-pod",
				Namespace: "my-namespace",
			},
			tags: []string{
				"kube_kind:Pod",
				"kube_name:my-pod",
				"kubernetes_kind:Pod",
				"name:my-pod",
				"kube_namespace:my-namespace",
				"namespace:my-namespace",
				"pod_name:my-pod",
			},
		},
		{
			name: "get pod namespace tags",
			involvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "my-pod",
				Namespace: "default",
			},
			tags: []string{
				"kube_kind:Pod",
				"kube_name:my-pod",
				"kubernetes_kind:Pod",
				"name:my-pod",
				"kube_namespace:default",
				"namespace:default",
				"team:container-int", // this tag is coming from the namespace
				"pod_name:my-pod",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, getInvolvedObjectTags(tt.involvedObject, taggerInstance), tt.tags)
		})
	}
}

func Test_getEventHostInfoImpl(t *testing.T) {
	providerIDFunc := func(clusterName string) string { return fmt.Sprintf("foo-%s", clusterName) }

	type args struct {
		clusterName string
		ev          *v1.Event
	}
	tests := []struct {
		name string
		args args
		want eventHostInfo
	}{
		{
			// the Kubelet source is providing the Host in the source section
			name: "node event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name: "my-node-1",
						Kind: nodeKind,
					},
					Source: v1.EventSource{
						Component: "kubelet",
						Host:      "my-node-1",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// other controller like `draino`, `cluster-autoscaler` doesn't set the host name in source.
			name: "node event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name: "my-node-1",
						Kind: nodeKind,
					},
					Source: v1.EventSource{
						Component: "draino",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// the Kubelet source is providing the Host in the source section
			name: "Pod event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name:      "my-pod-cdasd-adffd",
						Namespace: "foo",
						Kind:      podKind,
					},
					Source: v1.EventSource{
						Component: "kubelet",
						Host:      "my-node-1",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// other controller like draino don't set the host in the source section
			// for now the Nodename will be empty, but with workload meta in the cluster-agent
			// we should be able to retrieve the Node name.
			name: "Pod event from draino",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name:      "my-pod-cdasd-adffd",
						Namespace: "foo",
						Kind:      podKind,
					},
					Source: v1.EventSource{
						Component: "draino",
					},
				},
			},
			want: eventHostInfo{
				hostname: "",
				nodename: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getEventHostInfoImpl(providerIDFunc, tt.args.clusterName, tt.args.ev); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEventHostInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getEventSource(t *testing.T) {
	tests := []struct {
		name                                  string
		controllerName                        string
		sourceComponent                       string
		kubernetesEventSourceDetectionEnabled bool
		want                                  string
	}{
		{
			name:                                  "kubernetes event source detection maps controller name",
			kubernetesEventSourceDetectionEnabled: true,
			controllerName:                        "datadog-operator-manager",
			sourceComponent:                       "",
			want:                                  "datadog operator",
		},
		{
			name:                                  "kubernetes event source detection source component name",
			kubernetesEventSourceDetectionEnabled: true,
			controllerName:                        "",
			sourceComponent:                       "datadog-operator-manager",
			want:                                  "datadog operator",
		},
		{
			name:                                  "kubernetes event source detection uses default value if controller name not found",
			kubernetesEventSourceDetectionEnabled: true,
			controllerName:                        "abcd-test-controller",
			sourceComponent:                       "abcd-test-source",
			want:                                  "kubernetes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getEventSource(tt.controllerName, tt.sourceComponent); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEventSource() = %v, want %v", got, tt.want)
			}
		})
	}
}
