// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package proto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// This function tests both the function that converts a workloadmeta.Event into
// protobuf and the one that converts the protobuf into workloadmeta.Event. This
// is to avoid duplicating all the events and protobufs in 2 functions.
func TestConversions(t *testing.T) {
	createdAt := time.Unix(1669071600, 0)

	tests := []struct {
		name                   string
		workloadmetaEvent      workloadmeta.Event
		protoWorkloadmetaEvent *pb.WorkloadmetaEvent
		expectsError           bool
	}{
		{
			name: "event with a container",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "123",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "abc",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					EnvVars: map[string]string{
						"an_env": "an_env_val",
					},
					Hostname: "test_host",
					Image: workloadmeta.ContainerImage{
						ID:        "123",
						RawName:   "datadog/agent:7",
						Name:      "datadog/agent",
						ShortName: "agent",
						Tag:       "7",
					},
					NetworkIPs: map[string]string{
						"net1": "10.0.0.1",
						"net2": "192.168.0.1",
					},
					PID: 0,
					Ports: []workloadmeta.ContainerPort{
						{
							Port:     2000,
							Protocol: "tcp",
						},
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					State: workloadmeta.ContainerState{
						Running:    true,
						Status:     workloadmeta.ContainerStatusRunning,
						Health:     workloadmeta.ContainerHealthHealthy,
						CreatedAt:  createdAt,
						StartedAt:  createdAt,
						FinishedAt: time.Time{},
						ExitCode:   nil,
					},
					CollectorTags: []string{
						"tag1",
					},
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				Container: &pb.Container{
					EntityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_CONTAINER,
						Id:   "123",
					},
					EntityMeta: &pb.EntityMeta{
						Name:      "abc",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					EnvVars: map[string]string{
						"an_env": "an_env_val",
					},
					Hostname: "test_host",
					Image: &pb.ContainerImage{
						Id:        "123",
						RawName:   "datadog/agent:7",
						Name:      "datadog/agent",
						ShortName: "agent",
						Tag:       "7",
					},
					NetworkIps: map[string]string{
						"net1": "10.0.0.1",
						"net2": "192.168.0.1",
					},
					Pid: 0,
					Ports: []*pb.ContainerPort{
						{
							Port:     2000,
							Protocol: "tcp",
						},
					},
					Runtime: pb.Runtime_CONTAINERD,
					State: &pb.ContainerState{
						Running:    true,
						Status:     pb.ContainerStatus_CONTAINER_STATUS_RUNNING,
						Health:     pb.ContainerHealth_CONTAINER_HEALTH_HEALTHY,
						CreatedAt:  createdAt.Unix(),
						StartedAt:  createdAt.Unix(),
						FinishedAt: time.Time{}.Unix(),
						ExitCode:   0,
					},
					CollectorTags: []string{
						"tag1",
					},
				},
			},
			expectsError: false,
		},
		{
			name: "event with a Kubernetes pod",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesPod{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "123",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test_pod",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					Owners: []workloadmeta.KubernetesPodOwner{
						{
							Kind: kubernetes.DeploymentKind,
							Name: "test_deployment",
							ID:   "d1",
						},
					},
					PersistentVolumeClaimNames: []string{
						"pvc-0",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "fooID",
							Name: "fooName",
							Image: workloadmeta.ContainerImage{
								ID:        "123",
								RawName:   "datadog/agent:7",
								Name:      "datadog/agent",
								ShortName: "agent",
								Tag:       "7",
							},
						},
					},
					Ready:         true,
					Phase:         "Running",
					IP:            "127.0.0.1",
					PriorityClass: "some_priority",
					QOSClass:      "guaranteed",
					KubeServices: []string{
						"service1",
					},
					NamespaceLabels: map[string]string{
						"a_label": "a_label_value",
					},
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				KubernetesPod: &pb.KubernetesPod{
					EntityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_KUBERNETES_POD,
						Id:   "123",
					},
					EntityMeta: &pb.EntityMeta{
						Name:      "test_pod",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					Owners: []*pb.KubernetesPodOwner{
						{
							Kind: kubernetes.DeploymentKind,
							Name: "test_deployment",
							Id:   "d1",
						},
					},
					PersistentVolumeClaimNames: []string{
						"pvc-0",
					},
					Containers: []*pb.OrchestratorContainer{
						{
							Id:   "fooID",
							Name: "fooName",
							Image: &pb.ContainerImage{
								Id:        "123",
								RawName:   "datadog/agent:7",
								Name:      "datadog/agent",
								ShortName: "agent",
								Tag:       "7",
							},
						},
					},
					Ready:         true,
					Phase:         "Running",
					Ip:            "127.0.0.1",
					PriorityClass: "some_priority",
					QosClass:      "guaranteed",
					KubeServices: []string{
						"service1",
					},
					NamespaceLabels: map[string]string{
						"a_label": "a_label_value",
					},
				},
			},
			expectsError: false,
		},
		{
			name: "event with an ECS task",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.ECSTask{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindECSTask,
						ID:   "123",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "abc",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					Tags: map[string]string{
						"a_tag": "a_tag_value",
					},
					ContainerInstanceTags: map[string]string{
						"another_tag": "another_tag_value",
					},
					ClusterName:      "test_cluster",
					Region:           "some_region",
					AvailabilityZone: "some_az",
					Family:           "some_family",
					Version:          "1",
					LaunchType:       workloadmeta.ECSLaunchTypeEC2,
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "123",
							Name: "test_container",
							Image: workloadmeta.ContainerImage{
								ID:        "123",
								RawName:   "datadog/agent:7",
								Name:      "datadog/agent",
								ShortName: "agent",
								Tag:       "7",
							},
						},
					},
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				EcsTask: &pb.ECSTask{
					EntityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_ECS_TASK,
						Id:   "123",
					},
					EntityMeta: &pb.EntityMeta{
						Name:      "abc",
						Namespace: "default",
						Annotations: map[string]string{
							"an_annotation": "an_annotation_value",
						},
						Labels: map[string]string{
							"a_label": "a_label_value",
						},
					},
					Tags: map[string]string{
						"a_tag": "a_tag_value",
					},
					ContainerInstanceTags: map[string]string{
						"another_tag": "another_tag_value",
					},
					ClusterName:      "test_cluster",
					Region:           "some_region",
					AvailabilityZone: "some_az",
					Family:           "some_family",
					Version:          "1",
					LaunchType:       pb.ECSLaunchType_EC2,
					Containers: []*pb.OrchestratorContainer{
						{
							Id:   "123",
							Name: "test_container",
							Image: &pb.ContainerImage{
								Id:        "123",
								RawName:   "datadog/agent:7",
								Name:      "datadog/agent",
								ShortName: "agent",
								Tag:       "7",
							},
						},
					},
				},
			},
			expectsError: false,
		},
		{
			name: "invalid event",
			workloadmetaEvent: workloadmeta.Event{
				Type:   -1, // invalid
				Entity: &workloadmeta.Container{},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type:      -1, // invalid
				Container: &pb.Container{},
			},
			expectsError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultProtobuf, err := ProtobufEventFromWorkloadmetaEvent(test.workloadmetaEvent)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, proto.Equal(resultProtobuf, test.protoWorkloadmetaEvent))
			}

			resultWorkloadmetaEvent, err := WorkloadmetaEventFromProtoEvent(test.protoWorkloadmetaEvent)
			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.workloadmetaEvent, resultWorkloadmetaEvent)
			}
		})
	}
}

func TestWorkloadmetaFilterFromProtoFilter(t *testing.T) {
	protoFilter := pb.WorkloadmetaFilter{
		Kinds: []pb.WorkloadmetaKind{
			pb.WorkloadmetaKind_CONTAINER,
		},
		Source:    pb.WorkloadmetaSource_RUNTIME,
		EventType: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
	}

	resultFilter, err := WorkloadmetaFilterFromProtoFilter(&protoFilter)
	assert.NoError(t, err)

	assert.True(t, resultFilter.MatchKind(workloadmeta.KindContainer))
	assert.False(t, resultFilter.MatchKind(workloadmeta.KindKubernetesPod))
	assert.False(t, resultFilter.MatchKind(workloadmeta.KindECSTask))

	assert.Equal(t, workloadmeta.SourceRuntime, resultFilter.Source())
	assert.Equal(t, workloadmeta.EventTypeSet, resultFilter.EventType())
}
