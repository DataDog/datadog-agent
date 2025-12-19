// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package proto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
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
					ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
						{Name: "nvidia.com/gpu", ID: "gpu1"},
					},
					Resources: workloadmeta.ContainerResources{
						CPURequest:    pointer.Ptr(0.5),
						CPULimit:      pointer.Ptr(1.0),
						MemoryRequest: pointer.Ptr[uint64](1024),
						MemoryLimit:   pointer.Ptr[uint64](2048),
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
					ResolvedAllocatedResources: []*pb.ContainerAllocatedResource{
						{Name: "nvidia.com/gpu", ID: "gpu1"},
					},
					Resources: &pb.ContainerResources{
						CpuRequest:    pointer.Ptr(0.5),
						CpuLimit:      pointer.Ptr(1.0),
						MemoryRequest: pointer.Ptr[uint64](1024),
						MemoryLimit:   pointer.Ptr[uint64](2048),
					},
				},
			},
			expectsError: false,
		},
		{
			name: "event with a container with an owner",
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
					ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
						{Name: "nvidia.com/gpu", ID: "gpu1"},
					},
					Owner: &workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "pod123",
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
					ResolvedAllocatedResources: []*pb.ContainerAllocatedResource{
						{Name: "nvidia.com/gpu", ID: "gpu1"},
					},
					Owner: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_KUBERNETES_POD,
						Id:   "pod123",
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
					EphemeralContainers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "ephemeralID",
							Name: "ephemeralName",
							Image: workloadmeta.ContainerImage{
								ID:        "456",
								RawName:   "busybox:latest",
								Name:      "busybox",
								ShortName: "busybox",
								Tag:       "latest",
							},
						},
					},
					Ready:         true,
					Phase:         "Running",
					IP:            "127.0.0.1",
					PriorityClass: "some_priority",
					QOSClass:      "guaranteed",
					RuntimeClass:  "myclass",
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
					EphemeralContainers: []*pb.OrchestratorContainer{
						{
							Id:   "ephemeralID",
							Name: "ephemeralName",
							Image: &pb.ContainerImage{
								Id:        "456",
								RawName:   "busybox:latest",
								Name:      "busybox",
								ShortName: "busybox",
								Tag:       "latest",
							},
						},
					},
					Ready:         true,
					Phase:         "Running",
					Ip:            "127.0.0.1",
					PriorityClass: "some_priority",
					QosClass:      "guaranteed",
					RuntimeClass:  "myclass",
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
			name: "event with a process (minimal)",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:            1234,
					NsPid:          5678,
					Ppid:           1,
					Name:           "test_process",
					Cwd:            "/usr/bin",
					Exe:            "/usr/bin/test_process",
					Comm:           "test_proc",
					Cmdline:        []string{"test_process"},
					Uids:           []int32{1000},
					Gids:           []int32{1000},
					ContainerID:    "container123",
					CreationTime:   createdAt,
					InjectionState: workloadmeta.InjectionUnknown,
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				Process: &pb.Process{
					EntityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_PROCESS,
						Id:   "1234",
					},
					Pid:            1234,
					Nspid:          5678,
					Ppid:           1,
					Name:           "test_process",
					Cwd:            "/usr/bin",
					Exe:            "/usr/bin/test_process",
					Comm:           "test_proc",
					Cmdline:        []string{"test_process"},
					Uids:           []int32{1000},
					Gids:           []int32{1000},
					ContainerId:    "container123",
					CreationTime:   createdAt.Unix(),
					InjectionState: pb.InjectionState_INJECTION_UNKNOWN,
				},
			},
			expectsError: false,
		},
		{
			name: "event with a process (full)",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:          1234,
					NsPid:        5678,
					Ppid:         1,
					Name:         "test_process",
					Cwd:          "/usr/bin",
					Exe:          "/usr/bin/test_process",
					Comm:         "test_proc",
					Cmdline:      []string{"test_process", "--config", "/etc/config.yaml"},
					Uids:         []int32{0, 1000},
					Gids:         []int32{0, 1000},
					ContainerID:  "container123",
					CreationTime: createdAt,
					Language: &languagemodels.Language{
						Name:    languagemodels.Go,
						Version: "1.21.0",
					},
					InjectionState: workloadmeta.InjectionInjected,
					Owner: &workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container123",
					},
					Service: &workloadmeta.Service{
						GeneratedName:            "test_service",
						GeneratedNameSource:      "process_name",
						AdditionalGeneratedNames: []string{"alt_service_name"},
						TracerMetadata: []tracermetadata.TracerMetadata{
							{
								RuntimeID:   "runtime123",
								ServiceName: "test_service",
							},
						},
						TCPPorts:           []uint16{8080, 9090},
						UDPPorts:           []uint16{53},
						APMInstrumentation: true,
						UST: workloadmeta.UST{
							Service: "test_service",
							Env:     "test_env",
							Version: "test_version",
						},
					},
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				Process: &pb.Process{
					EntityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_PROCESS,
						Id:   "1234",
					},
					Pid:          1234,
					Nspid:        5678,
					Ppid:         1,
					Name:         "test_process",
					Cwd:          "/usr/bin",
					Exe:          "/usr/bin/test_process",
					Comm:         "test_proc",
					Cmdline:      []string{"test_process", "--config", "/etc/config.yaml"},
					Uids:         []int32{0, 1000},
					Gids:         []int32{0, 1000},
					ContainerId:  "container123",
					CreationTime: createdAt.Unix(),
					Language: &pb.Language{
						Name:    "go",
						Version: "1.21.0",
					},
					Owner: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_CONTAINER,
						Id:   "container123",
					},
					Service: &pb.Service{
						GeneratedName:            "test_service",
						GeneratedNameSource:      "process_name",
						AdditionalGeneratedNames: []string{"alt_service_name"},
						TracerMetadata: []*pb.TracerMetadata{
							{
								RuntimeId:   "runtime123",
								ServiceName: "test_service",
							},
						},
						TcpPorts:           []int32{8080, 9090},
						UdpPorts:           []int32{53},
						ApmInstrumentation: true,
						Ust: &pb.UST{
							Service: "test_service",
							Env:     "test_env",
							Version: "test_version",
						},
					},
					InjectionState: pb.InjectionState_INJECTION_INJECTED,
				},
			},
			expectsError: false,
		},
		{
			name: "event with valid crd",
			workloadmetaEvent: workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.CRD{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindCRD,
						ID:   "crd://datadogagents.datadoghq.com",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:        "datadogagents.datadoghq.com",
						Namespace:   "",
						Annotations: map[string]string{"meta.helm.sh/release-name": "datadog-operator"},
						Labels:      map[string]string{"pp.kubernetes.io/managed-by": "Helm"},
					},
					Group:   "datadoghq.com",
					Kind:    "DatadogAgent",
					Version: "v2alpha1e",
				},
			},
			protoWorkloadmetaEvent: &pb.WorkloadmetaEvent{
				Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
				Crd: &pb.Crd{
					EnityId: &pb.WorkloadmetaEntityId{
						Kind: pb.WorkloadmetaKind_CRD,
						Id:   "crd://datadogagents.datadoghq.com",
					},
					EntityMeta: &pb.EntityMeta{
						Name:        "datadogagents.datadoghq.com",
						Namespace:   "",
						Annotations: map[string]string{"meta.helm.sh/release-name": "datadog-operator"},
						Labels:      map[string]string{"pp.kubernetes.io/managed-by": "Helm"},
					},
					Group:   "datadoghq.com",
					Kind:    "DatadogAgent",
					Version: "v2alpha1e",
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

// This is added to test cases where some fields are unpopulated, resulting in asymmetric
// conversion (i.e. chaining both conversions doesn't yield an identity conversion)
func TestConvertWorkloadEventToProtoWithUnpopulatedFields(t *testing.T) {
	createdAt := time.Unix(1669071600, 0)

	wlmEvent := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "123",
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "abc",
				Namespace: "default",
			},
			Image: workloadmeta.ContainerImage{
				ID:        "123",
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
			},
			State: workloadmeta.ContainerState{
				Running:    true,
				CreatedAt:  createdAt,
				StartedAt:  createdAt,
				FinishedAt: time.Time{},
				ExitCode:   nil,
			},
		},
	}

	expectedProtoEvent := &pb.WorkloadmetaEvent{
		Type: pb.WorkloadmetaEventType_EVENT_TYPE_SET,
		Container: &pb.Container{
			EntityId: &pb.WorkloadmetaEntityId{
				Kind: pb.WorkloadmetaKind_CONTAINER,
				Id:   "123",
			},
			EntityMeta: &pb.EntityMeta{
				Name:      "abc",
				Namespace: "default",
			},
			Image: &pb.ContainerImage{
				Id:        "123",
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
			},
			Pid:     0,
			Runtime: pb.Runtime_UNKNOWN,
			State: &pb.ContainerState{
				Running:    true,
				Status:     pb.ContainerStatus_CONTAINER_STATUS_UNKNOWN,
				Health:     pb.ContainerHealth_CONTAINER_HEALTH_UNKNOWN,
				CreatedAt:  createdAt.Unix(),
				StartedAt:  createdAt.Unix(),
				FinishedAt: time.Time{}.Unix(),
				ExitCode:   0,
			},
		},
	}

	actualProtoEvent, err := ProtobufEventFromWorkloadmetaEvent(wlmEvent)
	assert.NoError(t, err)
	assert.Equal(t, expectedProtoEvent, actualProtoEvent)
}

func TestProtobufFilterFromWorkloadmetaFilter(t *testing.T) {
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceRuntime).
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindContainer).
		Build()

	protoFilter, err := ProtobufFilterFromWorkloadmetaFilter(filter)
	require.NoError(t, err)

	assert.ElementsMatch(t, []pb.WorkloadmetaKind{pb.WorkloadmetaKind_CONTAINER}, protoFilter.Kinds)
	assert.Equal(t, pb.WorkloadmetaSource_RUNTIME, protoFilter.Source)
	assert.Equal(t, pb.WorkloadmetaEventType_EVENT_TYPE_SET, protoFilter.EventType)
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
