// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func createEvent(count int32, namespace, objname, objkind, objuid, component, reportingController, hostname, reason, message, typ string, timestamp int64) *v1.Event {
	return &v1.Event{
		InvolvedObject: v1.ObjectReference{
			Name:      objname,
			Kind:      objkind,
			UID:       types.UID(objuid),
			Namespace: namespace,
		},
		Count: count,
		Source: v1.EventSource{
			Component: component,
			Host:      hostname,
		},
		Reason: reason,
		FirstTimestamp: metav1.Time{
			Time: time.Unix(timestamp, 0),
		},
		LastTimestamp: metav1.Time{
			Time: time.Unix(timestamp, 0),
		},
		Message:             message,
		Type:                typ,
		ReportingController: reportingController,
	}
}

func TestFormatEvent(t *testing.T) {
	objUID := "e6417a7f-f566-11e7-9749-0e4863e1cbf4"
	podName := "dca-789976f5d7-2ljx6"
	timestamp := int64(709662600)
	nodeName := "machine-blue"
	clusterName := "test-cluster"

	tests := []struct {
		name           string
		clusterName    string
		hostProviderID string
		events         []*v1.Event
		expected       event.Event
	}{
		{
			name: "basic event",
			events: []*v1.Event{
				createEvent(2, "default", podName, "Pod", objUID, "default-scheduler", "default-scheduler", nodeName, "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", timestamp),
				createEvent(3, "default", podName, "Pod", objUID, "default-scheduler", "default-scheduler", nodeName, "Started", "Started container", "Normal", timestamp),
			},
			expected: event.Event{
				Title:          "Events from the Pod default/dca-789976f5d7-2ljx6",
				Priority:       event.PriorityNormal,
				SourceTypeName: "kubernetes",
				EventType:      CheckName,
				Ts:             timestamp,
				Host:           nodeName,
				Tags: []string{
					"kube_namespace:default",
					"kube_kind:Pod",
					"kubernetes_kind:Pod",
					"namespace:default",
					"source_component:default-scheduler",
					"orchestrator:kubernetes",
					"reporting_controller:default-scheduler",
					fmt.Sprintf("kube_name:%s", podName),
					fmt.Sprintf("name:%s", podName),
					fmt.Sprintf("pod_name:%s", podName),
				},
				AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", objUID),
				Text: "%%% \n" + fmt.Sprintf(
					"%s \n _Events emitted by the %s seen at %s since %s_ \n",
					"2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n "+
						"3 **Started**: Started container\n",
					"default-scheduler",
					time.Unix(timestamp, 0),
					time.Unix(timestamp, 0),
				) + "\n %%%",
			},
		},
		{
			name: "event text escaping",
			events: []*v1.Event{
				createEvent(1, "default", podName, "Pod", objUID, "default-scheduler", "default-scheduler", nodeName, "Failed", "Error: error response: filepath: ~file~", "Normal", timestamp),
			},
			expected: event.Event{
				Title:          "Events from the Pod default/dca-789976f5d7-2ljx6",
				Priority:       event.PriorityNormal,
				SourceTypeName: "kubernetes",
				EventType:      CheckName,
				Ts:             timestamp,
				Host:           nodeName,
				Tags: []string{
					"kube_namespace:default",
					"kube_kind:Pod",
					"kubernetes_kind:Pod",
					"namespace:default",
					"source_component:default-scheduler",
					"orchestrator:kubernetes",
					"reporting_controller:default-scheduler",
					fmt.Sprintf("kube_name:%s", podName),
					fmt.Sprintf("name:%s", podName),
					fmt.Sprintf("pod_name:%s", podName),
				},
				AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", objUID),
				Text: "%%% \n" + fmt.Sprintf(
					"%s \n _Events emitted by the %s seen at %s since %s_ \n",
					"1 **Failed**: Error: error response: filepath: \\~file\\~\n",
					"default-scheduler",
					time.Unix(timestamp, 0),
					time.Unix(timestamp, 0),
				) + "\n %%%",
			},
		},
		{
			name:           "basic event with host info",
			clusterName:    clusterName,
			hostProviderID: "test-host-provider-id",
			events: []*v1.Event{
				createEvent(2, "default", podName, "Pod", objUID, "default-scheduler", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", timestamp),
				createEvent(3, "default", podName, "Pod", objUID, "default-scheduler", "default-scheduler", "machine-blue", "Started", "Started container", "Normal", timestamp),
			},
			expected: event.Event{
				Title:          "Events from the Pod default/dca-789976f5d7-2ljx6",
				Priority:       event.PriorityNormal,
				SourceTypeName: "kubernetes",
				EventType:      CheckName,
				Ts:             timestamp,
				Host:           fmt.Sprintf("%s-%s", nodeName, clusterName),
				Tags: []string{
					"kube_namespace:default",
					"kube_kind:Pod",
					"kubernetes_kind:Pod",
					"namespace:default",
					"source_component:default-scheduler",
					"host_provider_id:test-host-provider-id",
					"orchestrator:kubernetes",
					"reporting_controller:default-scheduler",
					fmt.Sprintf("kube_name:%s", podName),
					fmt.Sprintf("name:%s", podName),
					fmt.Sprintf("pod_name:%s", podName),
				},
				AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", objUID),
				Text: "%%% \n" + fmt.Sprintf(
					"%s \n _Events emitted by the %s seen at %s since %s_ \n",
					"2 **Scheduled**: Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54\n "+
						"3 **Started**: Started container\n",
					"default-scheduler",
					time.Unix(timestamp, 0),
					time.Unix(timestamp, 0),
				) + "\n %%%",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstEv := tt.events[0]

			hostProviderIDCache.Set(firstEv.Source.Host, tt.hostProviderID, cache.DefaultExpiration)
			defer hostProviderIDCache.Delete(firstEv.Source.Host)

			b := newKubernetesEventBundler(tt.clusterName, firstEv)

			for _, ev := range tt.events {
				b.addEvent(ev)
			}

			output, err := b.formatEvents(taggerfxmock.SetupFakeTagger(t))

			assert.Nil(t, err)
			assert.Equal(t, tt.expected.Text, output.Text)
			assert.Equal(t, tt.expected.Host, output.Host)
			assert.ElementsMatch(t, tt.expected.Tags, output.Tags)
		})
	}

}

func TestEventsTagging(t *testing.T) {
	tests := []struct {
		name         string
		k8sEvent     *v1.Event
		expectedTags []string
	}{
		{
			name:         "pod",
			k8sEvent:     createEvent(1, "default", "nginx-2d9jp-cmssw", "Pod", "c9f47d37-68d1-46a4-9295-419b054cb351", "kubelet", "kubelet", "xx-xx-default-pool-xxx-xxx", "Killing", "Stopping container daemon", "Normal", 709662600),
			expectedTags: []string{"source_component:kubelet", "kube_kind:Pod", "kubernetes_kind:Pod", "kube_name:nginx-2d9jp-cmssw", "name:nginx-2d9jp-cmssw", "pod_name:nginx-2d9jp-cmssw", "namespace:default", "kube_namespace:default", "reporting_controller:kubelet", "orchestrator:kubernetes"},
		},
		{
			name:         "deploy",
			k8sEvent:     createEvent(1, "default", "nginx", "Deployment", "b85978f5-2bf2-413f-9611-0b433d2cbf30", "deployment-controller", "deployment-controller", "", "ScalingReplicaSet", "Scaled up replica set nginx-b49f5958c to 1", "Normal", 709662600),
			expectedTags: []string{"source_component:deployment-controller", "kube_kind:Deployment", "kubernetes_kind:Deployment", "kube_name:nginx", "name:nginx", "kube_deployment:nginx", "namespace:default", "kube_namespace:default", "reporting_controller:deployment-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "replicaset",
			k8sEvent:     createEvent(1, "default", "nginx-b49f5958c", "ReplicaSet", "e048d70f-a83a-4559-9cc8-e55020c74ef0", "replicaset-controller", "replicaset-controller", "", "SuccessfulCreate", "Created pod: nginx-b49f5958c-cbwlk", "Normal", 709662600),
			expectedTags: []string{"source_component:replicaset-controller", "kube_kind:ReplicaSet", "kubernetes_kind:ReplicaSet", "kube_name:nginx-b49f5958c", "name:nginx-b49f5958c", "kube_replica_set:nginx-b49f5958c", "namespace:default", "kube_namespace:default", "reporting_controller:replicaset-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "cronjob",
			k8sEvent:     createEvent(1, "default", "logger", "CronJob", "5c3db67d-bba6-4322-b35d-0b6cb3adaf8b", "cronjob-controller", "cronjob-controller", "", "SuccessfulCreate", "Created job logger-160978308", "Normal", 709662600),
			expectedTags: []string{"source_component:cronjob-controller", "kube_kind:CronJob", "kubernetes_kind:CronJob", "kube_name:logger", "name:logger", "kube_cronjob:logger", "namespace:default", "kube_namespace:default", "reporting_controller:cronjob-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "job",
			k8sEvent:     createEvent(1, "default", "logger-1609783080", "Job", "8d8ae0d4-3e36-49be-94f5-786e823d7502", "job-controller", "job-controller", "", "SuccessfulCreate", "Created pod: logger-1609783080-5g2g4", "Normal", 709662600),
			expectedTags: []string{"source_component:job-controller", "kube_kind:Job", "kubernetes_kind:Job", "kube_name:logger-1609783080", "name:logger-1609783080", "kube_job:logger-1609783080", "namespace:default", "kube_namespace:default", "reporting_controller:job-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "service",
			k8sEvent:     createEvent(1, "default", "lb", "Service", "41f2f0fe-0ee1-4e98-a3c2-959093cf1016", "service-controller", "service-controller", "", "UpdatedLoadBalancer", "Updated load balancer with new hosts", "Normal", 709662600),
			expectedTags: []string{"source_component:service-controller", "kube_kind:Service", "kubernetes_kind:Service", "kube_name:lb", "name:lb", "kube_service:lb", "namespace:default", "kube_namespace:default", "reporting_controller:service-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "daemonset",
			k8sEvent:     createEvent(1, "default", "daemon", "DaemonSet", "764add75-7122-463c-9fde-241da14cf4e2", "daemonset-controller", "daemonset-controller", "", "SuccessfulCreate", "Created pod: daemon-8wr6f", "Normal", 709662600),
			expectedTags: []string{"source_component:daemonset-controller", "kube_kind:DaemonSet", "kubernetes_kind:DaemonSet", "kube_name:daemon", "name:daemon", "kube_daemon_set:daemon", "namespace:default", "kube_namespace:default", "reporting_controller:daemonset-controller", "orchestrator:kubernetes"},
		},
		{
			name:         "statefulset",
			k8sEvent:     createEvent(1, "default", "stateful", "StatefulSet", "493fc503-1264-418c-9af5-b8a961779194", "statefulset-controller", "statefulset-controller", "", "FailedCreate", "create Pod stateful-0 in StatefulSet stateful failed", "Warning", 709662600),
			expectedTags: []string{"source_component:statefulset-controller", "kube_kind:StatefulSet", "kubernetes_kind:StatefulSet", "kube_name:stateful", "name:stateful", "kube_stateful_set:stateful", "namespace:default", "kube_namespace:default", "reporting_controller:statefulset-controller", "orchestrator:kubernetes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := newKubernetesEventBundler("", tt.k8sEvent)
			bundle.addEvent(tt.k8sEvent)
			got, err := bundle.formatEvents(taggerfxmock.SetupFakeTagger(t))
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedTags, got.Tags)
		})
	}
}
