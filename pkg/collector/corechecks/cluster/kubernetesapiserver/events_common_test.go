// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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
	taggerInstance := taggerfxmock.SetupFakeTagger(t)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesPodUID, "nginx"), "workloadmeta-kubernetes_pod", nil, []string{"additional_pod_tag:nginx"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesDeployment, "workload-redis/my-deployment-1"), "workloadmeta-kubernetes_deployment", nil, []string{"deployment_tag:redis-1"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesDeployment, "default/my-deployment-2"), "workloadmeta-kubernetes_deployment", nil, []string{"deployment_tag:redis-2"}, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", "default"))), "workloadmeta-kubernetes_node", []string{"team:container-int"}, nil, nil, nil)
	taggerInstance.SetTags(types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("api-group", "resourcetypes", "default", "generic-resource"))), "workloadmeta-kubernetes_resource", []string{"generic_tag:generic-resource"}, nil, nil, nil)

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*apiv1.APIResourceList{
		{
			GroupVersion: "api-group/v1",
			APIResources: []apiv1.APIResource{
				{Kind: "ResourceType", Name: "resourcetypes"},
			},
		},
	}

	err := apiserver.InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		involvedObject v1.ObjectReference
		cronJob        string // value returned by the stubbed cronJob resolver
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
		{
			// A Job with a CronJob-like name suffix but no CronJob owner must
			// not get a kube_cronjob tag (the tag is resolved from the Job's
			// ownerReferences, not derived from its name).
			name: "get standalone job tags (no cronjob owner despite timestamp-like name)",
			involvedObject: v1.ObjectReference{
				Kind:      "Job",
				Name:      "my-job-29701140",
				Namespace: "default",
			},
			cronJob: "",
			tags: []string{
				"kube_kind:Job",
				"kube_name:my-job-29701140",
				"kubernetes_kind:Job",
				"name:my-job-29701140",
				"kube_namespace:default",
				"namespace:default",
				"team:container-int", // this tag is coming from the namespace
				"kube_job:my-job-29701140",
			},
		},
		{
			name: "get cronjob-owned job tags",
			involvedObject: v1.ObjectReference{
				Kind:      "Job",
				Name:      "my-cronjob-29701140",
				Namespace: "default",
			},
			cronJob: "my-cronjob",
			tags: []string{
				"kube_kind:Job",
				"kube_name:my-cronjob-29701140",
				"kubernetes_kind:Job",
				"name:my-cronjob-29701140",
				"kube_namespace:default",
				"namespace:default",
				"team:container-int", // this tag is coming from the namespace
				"kube_job:my-cronjob-29701140",
				"kube_cronjob:my-cronjob", // resolved from the job's ownerReferences
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolveCronJobForJob := func(_, _, _ string) string { return tt.cronJob }
			assert.ElementsMatch(t, getInvolvedObjectTagsImpl(tt.involvedObject, taggerInstance, resolveCronJobForJob), tt.tags)
		})
	}
}

func Test_getCronJobForJobWithClient(t *testing.T) {
	controller := true

	tests := []struct {
		name           string
		job            *batchv1.Job
		eventUID       string
		reactionErr    error
		wantCronJob    string
		wantDefinitive bool
	}{
		{
			name: "job owned by a cronjob",
			job: &batchv1.Job{
				ObjectMeta: apiv1.ObjectMeta{
					Name:      "my-cronjob-29701140",
					Namespace: "default",
					OwnerReferences: []apiv1.OwnerReference{
						{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", Controller: &controller},
					},
				},
			},
			wantCronJob:    "my-cronjob",
			wantDefinitive: true,
		},
		{
			name: "job UID matches event UID",
			job: &batchv1.Job{
				ObjectMeta: apiv1.ObjectMeta{
					Name:      "my-cronjob-29701140",
					Namespace: "default",
					UID:       "job-uid-1",
					OwnerReferences: []apiv1.OwnerReference{
						{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", Controller: &controller},
					},
				},
			},
			eventUID:       "job-uid-1",
			wantCronJob:    "my-cronjob",
			wantDefinitive: true,
		},
		{
			name: "job recreated with same name (UID mismatch)",
			job: &batchv1.Job{
				ObjectMeta: apiv1.ObjectMeta{
					Name:      "my-cronjob-29701140",
					Namespace: "default",
					UID:       "new-job-uid",
					OwnerReferences: []apiv1.OwnerReference{
						{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", Controller: &controller},
					},
				},
			},
			eventUID:       "old-job-uid",
			wantCronJob:    "",
			wantDefinitive: true,
		},
		{
			name: "job without owner",
			job: &batchv1.Job{
				ObjectMeta: apiv1.ObjectMeta{
					Name:      "standalone-job",
					Namespace: "default",
				},
			},
			wantCronJob:    "",
			wantDefinitive: true,
		},
		{
			name: "job owned by a CronJob kind from another API group",
			job: &batchv1.Job{
				ObjectMeta: apiv1.ObjectMeta{
					Name:      "custom-job",
					Namespace: "default",
					OwnerReferences: []apiv1.OwnerReference{
						{APIVersion: "custom.io/v1", Kind: "CronJob", Name: "custom-cronjob", Controller: &controller},
					},
				},
			},
			wantCronJob:    "",
			wantDefinitive: true,
		},
		{
			name:           "job not found",
			job:            nil,
			wantCronJob:    "",
			wantDefinitive: true,
		},
		{
			name:           "transient API error",
			job:            nil,
			reactionErr:    errors.New("server is unavailable"),
			wantCronJob:    "",
			wantDefinitive: false,
		},
		{
			name:           "forbidden error is definitive",
			job:            nil,
			reactionErr:    &apierrors.StatusError{ErrStatus: apiv1.Status{Reason: apiv1.StatusReasonForbidden}},
			wantCronJob:    "",
			wantDefinitive: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []runtime.Object
			if tt.job != nil {
				objects = append(objects, tt.job)
			}
			client := fakeclientset.NewClientset(objects...)
			if tt.reactionErr != nil {
				client.PrependReactor("get", "jobs", func(_ clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.reactionErr
				})
			}

			jobName := "my-job"
			if tt.job != nil {
				jobName = tt.job.Name
			}

			cronJob, definitive := getCronJobForJobWithClient(context.Background(), client, "default", jobName, tt.eventUID)
			assert.Equal(t, tt.wantCronJob, cronJob)
			assert.Equal(t, tt.wantDefinitive, definitive)
		})
	}
}

func Test_getEventHostInfoImpl(t *testing.T) {
	providerIDFunc := func(clusterName string) string { return "foo-" + clusterName }

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
		mockConfig.SetInTest("kubernetes_events_source_detection.enabled", tt.kubernetesEventSourceDetectionEnabled)
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
