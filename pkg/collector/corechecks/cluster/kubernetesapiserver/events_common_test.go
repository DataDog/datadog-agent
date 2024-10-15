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
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
	telemetryComponent := fxutil.Test[coretelemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := telemetry.NewStore(telemetryComponent)
	cfg := configmock.New(t)
	taggerInstance := local.NewFakeTagger(cfg, telemetryStore)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesPodUID, "nginx"), "workloadmeta-kubernetes_pod", nil, []string{"additional_pod_tag:nginx"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesDeployment, "workload-redis/my-deployment-1"), "workloadmeta-kubernetes_deployment", nil, []string{"deployment_tag:redis-1"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesDeployment, "default/my-deployment-2"), "workloadmeta-kubernetes_deployment", nil, []string{"deployment_tag:redis-2"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", "default"))), "workloadmeta-kubernetes_node", []string{"team:container-int"}, nil, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("api-group", "resourcetypes", "default", "generic-resource"))), "workloadmeta-kubernetes_resource", []string{"generic_tag:generic-resource"}, nil, nil, nil)

	tests := []struct {
		name           string
		involvedObject v1.ObjectReference
		tags           []string
	}{
		{
			name: "get pod basic tags",
			involvedObject: v1.ObjectReference{
				UID:       "nginx",
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
				"additional_pod_tag:nginx",
			},
		},
		{
			name: "get pod namespace tags",
			involvedObject: v1.ObjectReference{
				UID:       "nginx",
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
				"additional_pod_tag:nginx",
			},
		},
		{
			name: "get deployment basic tags",
			involvedObject: v1.ObjectReference{
				Kind:      "Deployment",
				Name:      "my-deployment-1",
				Namespace: "workload-redis",
			},
			tags: []string{
				"kube_kind:Deployment",
				"kube_name:my-deployment-1",
				"kubernetes_kind:Deployment",
				"name:my-deployment-1",
				"kube_namespace:workload-redis",
				"namespace:workload-redis",
				"kube_deployment:my-deployment-1",
				"deployment_tag:redis-1",
			},
		},
		{
			name: "get deployment namespace tags",
			involvedObject: v1.ObjectReference{
				Kind:      "Deployment",
				Name:      "my-deployment-2",
				Namespace: "default",
			},
			tags: []string{
				"kube_kind:Deployment",
				"kube_name:my-deployment-2",
				"kubernetes_kind:Deployment",
				"name:my-deployment-2",
				"kube_namespace:default",
				"namespace:default",
				"kube_deployment:my-deployment-2",
				"team:container-int", // this tag is coming from the namespace
				"deployment_tag:redis-2",
			},
		},
		{
			name: "get tags for any metadata resource",
			involvedObject: v1.ObjectReference{
				Kind:       "ResourceType",
				Name:       "generic-resource",
				Namespace:  "default",
				APIVersion: "api-group/v1",
			},
			tags: []string{
				"kube_kind:ResourceType",
				"kube_name:generic-resource",
				"kubernetes_kind:ResourceType",
				"name:generic-resource",
				"kube_namespace:default",
				"namespace:default",
				"team:container-int", // this tag is coming from the namespace
				"generic_tag:generic-resource",
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
		{
			name:                                  "kubernetes event source detection uses default value if source detection disabled",
			kubernetesEventSourceDetectionEnabled: false,
			controllerName:                        "datadog-operator-manager",
			sourceComponent:                       "datadog-operator-manager",
			want:                                  "kubernetes",
		},
	}
	for _, tt := range tests {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("kubernetes_events_source_detection.enabled", tt.kubernetesEventSourceDetectionEnabled)
		t.Run(tt.name, func(t *testing.T) {
			if got := getEventSource(tt.controllerName, tt.sourceComponent); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEventSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_shouldCollect(t *testing.T) {
	tests := []struct {
		name           string
		ev             *v1.Event
		collectedTypes []collectedEventType
		shouldCollect  bool
	}{
		{
			name: "kubernetes event collection matches based on kind",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "my-node-1",
				},
			},
			collectedTypes: []collectedEventType{
				{
					Kind: "Pod",
				},
			},
			shouldCollect: true,
		},
		{
			name: "kubernetes event collection matches based on source",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "my-node-1",
				},
			},
			collectedTypes: []collectedEventType{
				{
					Source: "kubelet",
				},
			},
			shouldCollect: true,
		},
		{
			name: "kubernetes event collection matches based on reason",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "my-node-1",
				},
				Reason: "CrashLoopBackOff",
			},
			collectedTypes: []collectedEventType{
				{
					Reasons: []string{"CrashLoopBackOff"},
				},
			},
			shouldCollect: true,
		},
		{
			name: "kubernetes event collection matches by kind and reason",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "my-node-1",
				},
				Reason: "CrashLoopBackOff",
			},
			collectedTypes: []collectedEventType{
				{
					Kind:    "Pod",
					Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
				},
				{
					Kind:    "Node",
					Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
				},
				{
					Kind:    "CronJob",
					Reasons: []string{"SawCompletedJob"},
				},
				{
					Reasons: []string{"CrashLoopBackOff"},
				},
			},
			shouldCollect: true,
		},
		{
			name: "kubernetes event collection matches by source and reason",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "my-node-1",
				},
				Reason: "CrashLoopBackOff",
			},
			collectedTypes: []collectedEventType{
				{
					Source:  "kubelet",
					Reasons: []string{"CrashLoopBackOff"},
				},
			},
			shouldCollect: true,
		},
		{
			name: "kubernetes event collection matches none",
			ev: &v1.Event{
				InvolvedObject: v1.ObjectReference{
					Name: "my-pod-1",
					Kind: podKind,
				},
				Source: v1.EventSource{
					Component: "kubelet",
				},
				Reason: "something",
			},
			collectedTypes: []collectedEventType{
				{
					Source:  "kubelet",
					Reasons: []string{"CrashLoopBackOff"},
				},
			},
			shouldCollect: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldCollect, shouldCollect(tt.ev, tt.collectedTypes))
		})
	}
}
