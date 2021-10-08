// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"

	cache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestFormatEvent(t *testing.T) {
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", 709662600)
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", "Normal", 709662600)

	eventList := []*v1.Event{
		ev1,
		ev2,
	}
	b := &kubernetesEventBundle{
		name:          "dca-789976f5d7-2ljx6",
		events:        eventList,
		objUID:        types.UID("some_id"),
		component:     "Pod",
		kind:          "Pod",
		countByAction: make(map[string]int),
	}

	expectedOutput := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", b.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           "",
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", b.component), fmt.Sprintf("kubernetes_kind:%s", b.kind), fmt.Sprintf("name:%s", b.name), fmt.Sprintf("pod_name:%s", b.name)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.objUID),
	}
	expectedOutput.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s since %s_ \n", formatStringIntMap(b.countByAction), b.component, time.Unix(int64(b.lastTimestamp), 0), time.Unix(int64(b.timeStamp), 0)) + "\n %%%"

	providerIDCache := cache.New(defaultCacheExpire, defaultCachePurge)
	output, err := b.formatEvents("", providerIDCache)

	assert.Nil(t, err, "not nil")
	assert.Equal(t, expectedOutput.Text, output.Text)
	assert.ElementsMatch(t, expectedOutput.Tags, output.Tags)
}

func TestFormatEventWithNodename(t *testing.T) {
	ev1 := createEvent(2, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Scheduled", "Successfully assigned dca-789976f5d7-2ljx6 to ip-10-0-0-54", "Normal", 709662600)
	ev2 := createEvent(3, "default", "dca-789976f5d7-2ljx6", "Pod", "e6417a7f-f566-11e7-9749-0e4863e1cbf4", "default-scheduler", "machine-blue", "Started", "Started container", "Normal", 709662600)

	eventList := []*v1.Event{
		ev1,
		ev2,
	}

	clusterName := "test_cluster"
	nodename := "test_nodename"
	providerID := "test_provider_ID"

	b := &kubernetesEventBundle{
		events:        eventList,
		objUID:        types.UID("some_id"),
		component:     "Pod",
		kind:          "Pod",
		name:          "dca-789976f5d7-2ljx6",
		countByAction: make(map[string]int),
		nodename:      nodename,
	}

	expectedOutput := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", b.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           nodename + "-" + clusterName,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", b.component), fmt.Sprintf("pod_name:%s", b.name), fmt.Sprintf("kubernetes_kind:%s", b.kind), fmt.Sprintf("name:%s", b.name), fmt.Sprintf("host_provider_id:%s", providerID)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.objUID),
	}
	expectedOutput.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s since %s_ \n", formatStringIntMap(b.countByAction), b.component, time.Unix(int64(b.lastTimestamp), 0), time.Unix(int64(b.timeStamp), 0)) + "\n %%%"

	providerIDCache := cache.New(defaultCacheExpire, defaultCachePurge)
	providerIDCache.Set(nodename, providerID, cache.NoExpiration)
	output, err := b.formatEvents(clusterName, providerIDCache)

	assert.Nil(t, err, "not nil")
	assert.Equal(t, expectedOutput.Text, output.Text)
	assert.ElementsMatch(t, expectedOutput.Tags, output.Tags)
}

func Test_getDDAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    metrics.EventAlertType
	}{
		{
			name:    "normal",
			k8sType: "Normal",
			want:    metrics.EventAlertTypeInfo,
		},
		{
			name:    "warning",
			k8sType: "Warning",
			want:    metrics.EventAlertTypeWarning,
		},
		{
			name:    "unknown",
			k8sType: "Unknown",
			want:    metrics.EventAlertTypeInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDDAlertType(tt.k8sType); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getDDAlertType() = %v, want %v", got, tt.want)
			}
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
			k8sEvent:     createEvent(1, "default", "nginx-2d9jp-cmssw", "Pod", "c9f47d37-68d1-46a4-9295-419b054cb351", "kubelet", "xx-xx-default-pool-xxx-xxx", "Killing", "Stopping container daemon", "Normal", 709662600),
			expectedTags: []string{"source_component:kubelet", "kubernetes_kind:Pod", "name:nginx-2d9jp-cmssw", "pod_name:nginx-2d9jp-cmssw", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "deploy",
			k8sEvent:     createEvent(1, "default", "nginx", "Deployment", "b85978f5-2bf2-413f-9611-0b433d2cbf30", "deployment-controller", "", "ScalingReplicaSet", "Scaled up replica set nginx-b49f5958c to 1", "Normal", 709662600),
			expectedTags: []string{"source_component:deployment-controller", "kubernetes_kind:Deployment", "name:nginx", "kube_deployment:nginx", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "replicaset",
			k8sEvent:     createEvent(1, "default", "nginx-b49f5958c", "ReplicaSet", "e048d70f-a83a-4559-9cc8-e55020c74ef0", "replicaset-controller", "", "SuccessfulCreate", "Created pod: nginx-b49f5958c-cbwlk", "Normal", 709662600),
			expectedTags: []string{"source_component:replicaset-controller", "kubernetes_kind:ReplicaSet", "name:nginx-b49f5958c", "kube_replica_set:nginx-b49f5958c", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "cronjob",
			k8sEvent:     createEvent(1, "default", "logger", "CronJob", "5c3db67d-bba6-4322-b35d-0b6cb3adaf8b", "cronjob-controller", "", "SuccessfulCreate", "Created job logger-160978308", "Normal", 709662600),
			expectedTags: []string{"source_component:cronjob-controller", "kubernetes_kind:CronJob", "name:logger", "kube_cronjob:logger", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "job",
			k8sEvent:     createEvent(1, "default", "logger-1609783080", "Job", "8d8ae0d4-3e36-49be-94f5-786e823d7502", "job-controller", "", "SuccessfulCreate", "Created pod: logger-1609783080-5g2g4", "Normal", 709662600),
			expectedTags: []string{"source_component:job-controller", "kubernetes_kind:Job", "name:logger-1609783080", "kube_job:logger-1609783080", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "service",
			k8sEvent:     createEvent(1, "default", "lb", "Service", "41f2f0fe-0ee1-4e98-a3c2-959093cf1016", "service-controller", "", "UpdatedLoadBalancer", "Updated load balancer with new hosts", "Normal", 709662600),
			expectedTags: []string{"source_component:service-controller", "kubernetes_kind:Service", "name:lb", "kube_service:lb", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "daemonset",
			k8sEvent:     createEvent(1, "default", "daemon", "DaemonSet", "764add75-7122-463c-9fde-241da14cf4e2", "daemonset-controller", "", "SuccessfulCreate", "Created pod: daemon-8wr6f", "Normal", 709662600),
			expectedTags: []string{"source_component:daemonset-controller", "kubernetes_kind:DaemonSet", "name:daemon", "kube_daemon_set:daemon", "namespace:default", "kube_namespace:default"},
		},
		{
			name:         "statefulset",
			k8sEvent:     createEvent(1, "default", "stateful", "StatefulSet", "493fc503-1264-418c-9af5-b8a961779194", "statefulset-controller", "", "FailedCreate", "create Pod stateful-0 in StatefulSet stateful failed", "Warning", 709662600),
			expectedTags: []string{"source_component:statefulset-controller", "kubernetes_kind:StatefulSet", "name:stateful", "kube_stateful_set:stateful", "namespace:default", "kube_namespace:default"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := newKubernetesEventBundler(tt.k8sEvent)
			bundle.addEvent(tt.k8sEvent)
			got, err := bundle.formatEvents("", cache.New(defaultCacheExpire, defaultCachePurge))
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedTags, got.Tags)
		})
	}
}
