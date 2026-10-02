// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestParsePod(t *testing.T) {
	creationTimestamp := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	conditionTransitionTime := creationTimestamp.Add(30 * time.Second)
	controller := true

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "test-namespace",
			UID:               types.UID("pod-uid"),
			CreationTimestamp: metav1.NewTime(creationTimestamp),
			Annotations:       map[string]string{"annotationKey": "annotationValue"},
			Labels:            map[string]string{"labelKey": "labelValue"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "deployment-hashrs",
					UID:        types.UID("owner-uid"),
					Controller: &controller,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName:          "test-node",
			HostNetwork:       true,
			PriorityClassName: "priorityClass",
			Volumes: []corev1.Volume{
				{
					Name: "pvcVol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvcName",
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{Name: "init-container-name", Image: "busybox:latest"},
			},
			Containers: []corev1.Container{
				{
					Name:  "nginx-container",
					Image: "nginx:1.25.2",
					Env: []corev1.EnvVar{
						{Name: "DD_ENV", Value: "prod"},
						{Name: "DD_SERVICE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("1"),
						},
					},
					ResizePolicy: []corev1.ContainerResizePolicy{
						{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.NotRequired},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:    corev1.PodRunning,
			HostIP:   "192.168.1.10",
			PodIP:    "127.0.0.1",
			QOSClass: corev1.PodQOSGuaranteed,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(conditionTransitionTime),
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "init-container-name",
					Image:       "busybox:latest",
					ImageID:     "sha256:abcd1234",
					ContainerID: "docker://init-containerID",
					State:       corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "nginx-container",
					Image:       "nginx:1.25.2",
					ImageID:     "5dbe7e1b6b9c",
					ContainerID: "docker://containerID",
					Ready:       true,
					State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}

	events := parsePod(pod, false)

	entities := make(map[workloadmeta.Kind][]workloadmeta.Entity)
	for _, event := range events {
		assert.Equal(t, workloadmeta.EventTypeSet, event.Type)
		assert.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
		entities[event.Entity.GetID().Kind] = append(entities[event.Entity.GetID().Kind], event.Entity)
	}

	require.Len(t, entities[workloadmeta.KindContainer], 2)
	require.Len(t, entities[workloadmeta.KindKubernetesPod], 1)

	podEntity := entities[workloadmeta.KindKubernetesPod][0].(*workloadmeta.KubernetesPod)
	podEntityID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"}

	assert.Equal(t, "pod-uid", podEntity.ID)
	assert.Equal(t, "test-pod", podEntity.Name)
	assert.Equal(t, "test-namespace", podEntity.Namespace)
	assert.Equal(t, map[string]string{"annotationKey": "annotationValue"}, podEntity.Annotations)
	assert.Equal(t, map[string]string{"labelKey": "labelValue"}, podEntity.Labels)
	assert.True(t, podEntity.Ready)
	require.NotNil(t, podEntity.ReadyTimestamp)
	assert.Equal(t, conditionTransitionTime, *podEntity.ReadyTimestamp)
	assert.Nil(t, podEntity.DeletionTimestamp)
	assert.Equal(t, "Running", podEntity.Phase)
	assert.Equal(t, "127.0.0.1", podEntity.IP)
	assert.Equal(t, "192.168.1.10", podEntity.HostIP)
	assert.True(t, podEntity.HostNetwork)
	assert.Equal(t, "priorityClass", podEntity.PriorityClass)
	assert.Equal(t, "Guaranteed", podEntity.QOSClass)
	assert.Equal(t, "test-node", podEntity.NodeName)
	assert.Equal(t, []string{"nvidia"}, podEntity.GPUVendorList)
	assert.Equal(t, []string{"pvcName"}, podEntity.PersistentVolumeClaimNames)
	assert.Equal(t, creationTimestamp, podEntity.CreationTimestamp)
	assert.Equal(t, []workloadmeta.KubernetesPodOwner{
		{Kind: "ReplicaSet", Name: "deployment-hashrs", ID: "owner-uid", Group: "apps", Controller: &controller},
	}, podEntity.Owners)

	require.Len(t, podEntity.Containers, 1)
	assert.Equal(t, "containerID", podEntity.Containers[0].ID)
	assert.Equal(t, workloadmeta.ContainerResources{GPUVendorList: []string{"nvidia"}}, podEntity.Containers[0].Resources)
	assert.Equal(t, workloadmeta.ContainerResizePolicy{CPURestartPolicy: string(corev1.NotRequired)}, podEntity.Containers[0].ResizePolicy)

	require.Len(t, podEntity.InitContainers, 1)
	assert.Equal(t, "init-containerID", podEntity.InitContainers[0].ID)

	var container *workloadmeta.Container
	for _, e := range entities[workloadmeta.KindContainer] {
		c := e.(*workloadmeta.Container)
		if c.ID == "containerID" {
			container = c
		}
	}
	require.NotNil(t, container)
	assert.Equal(t, "nginx-container", container.Name)
	assert.Equal(t, map[string]string{kubernetes.CriContainerNamespaceLabel: "test-namespace"}, container.Labels)
	assert.Equal(t, workloadmeta.ContainerRuntime("docker"), container.Runtime)
	assert.Equal(t, map[string]string{"DD_ENV": "prod"}, container.EnvVars)
	assert.True(t, container.State.Running)
	assert.Equal(t, workloadmeta.ContainerStatusRunning, container.State.Status)
	assert.Equal(t, workloadmeta.ContainerHealthHealthy, container.State.Health)
	assert.Equal(t, &podEntityID, container.Owner)
	assert.Equal(t, workloadmeta.ContainerImage{
		ID:        "5dbe7e1b6b9c",
		RawName:   "nginx:1.25.2",
		Name:      "nginx",
		ShortName: "nginx",
		Tag:       "1.25.2",
	}, container.Image)
}

// TestParsePodContainers_PrefersSpecImage verifies that the container spec's
// image, not the status's, is used for the Container entity and its
// OrchestratorContainer reference, mirroring the kubelet collector's own
// preference for the spec's image over the status's.
func TestParsePodContainers_PrefersSpecImage(t *testing.T) {
	specs := []corev1.Container{
		{Name: "nginx-container", Image: "nginx:1.25.3"},
	}
	statuses := []corev1.ContainerStatus{
		{
			Name:        "nginx-container",
			Image:       "nginx:1.25.2",
			ImageID:     "5dbe7e1b6b9c",
			ContainerID: "docker://containerID",
		},
	}

	podContainers, events := parsePodContainers(specs, statuses, "default", nil)

	wantImage := workloadmeta.ContainerImage{
		ID:        "5dbe7e1b6b9c",
		RawName:   "nginx:1.25.3",
		Name:      "nginx",
		ShortName: "nginx",
		Tag:       "1.25.3",
	}

	require.Len(t, podContainers, 1)
	assert.Equal(t, wantImage, podContainers[0].Image)

	require.Len(t, events, 1)
	container := events[0].Entity.(*workloadmeta.Container)
	assert.Equal(t, wantImage, container.Image)
}

// TestParsePodContainers_UnparsableSpecImage verifies that the status's image
// is kept when the spec's image can't be parsed, instead of being replaced by
// a half-populated image.
func TestParsePodContainers_UnparsableSpecImage(t *testing.T) {
	specs := []corev1.Container{
		{Name: "nginx-container", Image: ""},
	}
	statuses := []corev1.ContainerStatus{
		{
			Name:        "nginx-container",
			Image:       "nginx:1.25.2",
			ImageID:     "5dbe7e1b6b9c",
			ContainerID: "docker://containerID",
		},
	}

	podContainers, events := parsePodContainers(specs, statuses, "default", nil)

	wantImage := workloadmeta.ContainerImage{
		ID:        "5dbe7e1b6b9c",
		RawName:   "nginx:1.25.2",
		Name:      "nginx",
		ShortName: "nginx",
		Tag:       "1.25.2",
	}

	require.Len(t, podContainers, 1)
	assert.Equal(t, wantImage, podContainers[0].Image)

	require.Len(t, events, 1)
	container := events[0].Entity.(*workloadmeta.Container)
	assert.Equal(t, wantImage, container.Image)
}

// TestParsePodContainers_Ports verifies that the ports a container spec
// declares end up on its Container entity, as with the kubelet collector: the
// tagger resolves the %%port%% template variables of autodiscovery tag
// annotations from them.
func TestParsePodContainers_Ports(t *testing.T) {
	specs := []corev1.Container{
		{
			Name: "web",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
				{ContainerPort: 53, Protocol: corev1.ProtocolUDP},
			},
		},
		{Name: "sidecar"},
	}
	statuses := []corev1.ContainerStatus{
		{Name: "web", ContainerID: "docker://web-containerID"},
		{Name: "sidecar", ContainerID: "docker://sidecar-containerID"},
		{Name: "without-spec", ContainerID: "docker://without-spec-containerID"},
	}

	_, events := parsePodContainers(specs, statuses, "default", nil)

	require.Len(t, events, 3)
	ports := make(map[string][]workloadmeta.ContainerPort)
	for _, event := range events {
		container := event.Entity.(*workloadmeta.Container)
		ports[container.Name] = container.Ports
	}

	assert.Equal(t, []workloadmeta.ContainerPort{
		{Name: "http", Port: 8080, Protocol: "TCP"},
		{Port: 53, Protocol: "UDP"},
	}, ports["web"])
	assert.Empty(t, ports["sidecar"])
	assert.Empty(t, ports["without-spec"])
}

func TestParsePod_SkipsContainerWithoutID(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", UID: types.UID("pod-uid")},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "not-yet-created"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "not-yet-created", ContainerID: ""},
			},
		},
	}

	events := parsePod(pod, false)

	for _, event := range events {
		assert.NotEqual(t, workloadmeta.KindContainer, event.Entity.GetID().Kind)
	}
}

// TestParsePod_StaticPod verifies that a static pod, which the API server
// only holds as a mirror pod under another UID, is stored under the static
// pod's own UID, as the kubelet collector stores it, and that its containers
// point to it under that UID.
func TestParsePod_StaticPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "kube-apiserver-node",
			Namespace:   "kube-system",
			UID:         types.UID("85a6cc02-4460-4f8a-b5f0-0123456789ab"),
			Annotations: map[string]string{mirrorPodAnnotation: "9b3c1a2d4e5f60718293a4b5c6d7e8f9"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "kube-apiserver"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "kube-apiserver", ContainerID: "containerd://container-id"},
			},
		},
	}

	events := parsePod(pod, false)

	require.Len(t, events, 2)
	container := events[0].Entity.(*workloadmeta.Container)
	podEntity := events[1].Entity.(*workloadmeta.KubernetesPod)
	assert.Equal(t, "9b3c1a2d4e5f60718293a4b5c6d7e8f9", podEntity.ID)
	require.NotNil(t, container.Owner)
	assert.Equal(t, podEntity.EntityID, *container.Owner)
}

// TestParsePod_StaticPodOwners verifies that the node owner the kubelet gives a
// mirror pod isn't reported as an owner of the static pod it stands for, which
// has none for the kubelet collector, and that it's only left out for mirror
// pods.
func TestParsePod_StaticPodOwners(t *testing.T) {
	controller := true
	nodeOwner := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       "test-node",
		UID:        types.UID("node-uid"),
		Controller: &controller,
	}
	daemonSetOwner := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-daemonset",
		UID:        types.UID("daemonset-uid"),
	}
	mirrorPodAnnotations := map[string]string{mirrorPodAnnotation: "9b3c1a2d4e5f60718293a4b5c6d7e8f9"}

	tests := []struct {
		name        string
		annotations map[string]string
		owners      []metav1.OwnerReference
		want        []workloadmeta.KubernetesPodOwner
	}{
		{
			name:        "mirror pod without its node owner",
			annotations: mirrorPodAnnotations,
			owners:      []metav1.OwnerReference{nodeOwner},
			want:        nil,
		},
		{
			name:        "mirror pod keeps its other owners",
			annotations: mirrorPodAnnotations,
			owners:      []metav1.OwnerReference{nodeOwner, daemonSetOwner},
			want: []workloadmeta.KubernetesPodOwner{
				{Kind: "DaemonSet", Name: "test-daemonset", ID: "daemonset-uid", Group: "apps"},
			},
		},
		{
			name:   "regular pod keeps a node owner",
			owners: []metav1.OwnerReference{nodeOwner},
			want: []workloadmeta.KubernetesPodOwner{
				{Kind: "Node", Name: "test-node", ID: "node-uid", Controller: &controller},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kube-apiserver-test-node",
					Namespace:       "kube-system",
					UID:             types.UID("85a6cc02-4460-4f8a-b5f0-0123456789ab"),
					Annotations:     tt.annotations,
					OwnerReferences: tt.owners,
				},
			}

			events := parsePod(pod, false)

			require.Len(t, events, 1)
			podEntity := events[0].Entity.(*workloadmeta.KubernetesPod)
			assert.ElementsMatch(t, tt.want, podEntity.Owners)
		})
	}
}

func TestParsePod_DeletionTimestamp(t *testing.T) {
	deletionTime := metav1.NewTime(time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			UID:               types.UID("pod-uid"),
			DeletionTimestamp: &deletionTime,
		},
	}

	events := parsePod(pod, false)

	require.Len(t, events, 1)
	podEntity := events[0].Entity.(*workloadmeta.KubernetesPod)
	require.NotNil(t, podEntity.DeletionTimestamp)
	assert.Equal(t, deletionTime.Time, *podEntity.DeletionTimestamp)
}

// TestParsePod_ReadyWithoutTransitionTime verifies that a Ready pod whose
// Ready condition carries no transition time gets a nil ReadyTimestamp rather
// than a zero-valued one.
func TestParsePod_ReadyWithoutTransitionTime(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", UID: types.UID("pod-uid")},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	events := parsePod(pod, false)

	require.Len(t, events, 1)
	podEntity := events[0].Entity.(*workloadmeta.KubernetesPod)
	assert.True(t, podEntity.Ready)
	assert.Nil(t, podEntity.ReadyTimestamp)
}

// TestParsePod_EphemeralContainers verifies that ephemeral containers are
// only parsed into events and into KubernetesPod.EphemeralContainers when
// collectEphemeralContainers is set, mirroring the kubelet collector's own
// include_ephemeral_containers gate.
func TestParsePod_EphemeralContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", UID: types.UID("pod-uid")},
		Spec: corev1.PodSpec{
			EphemeralContainers: []corev1.EphemeralContainer{
				{
					EphemeralContainerCommon: corev1.EphemeralContainerCommon{
						Name:  "debugger",
						Image: "busybox:1.36",
					},
				},
			},
		},
		Status: corev1.PodStatus{
			EphemeralContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "debugger",
					Image:       "busybox:1.36",
					ImageID:     "5dbe7e1b6b9c",
					ContainerID: "docker://debugger-containerID",
					State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}

	t.Run("disabled", func(t *testing.T) {
		events := parsePod(pod, false)

		podEntity := events[len(events)-1].Entity.(*workloadmeta.KubernetesPod)
		assert.Empty(t, podEntity.EphemeralContainers)
		for _, event := range events {
			assert.NotEqual(t, "debugger-containerID", event.Entity.GetID().ID)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		events := parsePod(pod, true)

		podEntity := events[len(events)-1].Entity.(*workloadmeta.KubernetesPod)
		require.Len(t, podEntity.EphemeralContainers, 1)
		assert.Equal(t, "debugger-containerID", podEntity.EphemeralContainers[0].ID)

		var found bool
		for _, event := range events {
			if event.Entity.GetID().ID == "debugger-containerID" {
				found = true
				container := event.Entity.(*workloadmeta.Container)
				assert.Equal(t, "debugger", container.Name)
			}
		}
		assert.True(t, found, "expected a Container event for the ephemeral container")
	})
}

func TestExtractResizePolicy(t *testing.T) {
	spec := &corev1.Container{
		ResizePolicy: []corev1.ContainerResizePolicy{
			{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.RestartContainer},
			{ResourceName: corev1.ResourceMemory, RestartPolicy: corev1.NotRequired},
		},
	}

	assert.Equal(t, workloadmeta.ContainerResizePolicy{
		CPURestartPolicy:    string(corev1.RestartContainer),
		MemoryRestartPolicy: string(corev1.NotRequired),
	}, extractResizePolicy(spec))
}

func TestGpuVendorsFromLimits(t *testing.T) {
	limits := corev1.ResourceList{
		"nvidia.com/gpu": resource.MustParse("1"),
		"amd.com/gpu":    resource.MustParse("1"),
		"cpu":            resource.MustParse("100m"),
	}

	vendors := gpuVendorsFromLimits(limits)
	assert.Equal(t, []string{"amd", "nvidia"}, vendors)
}

func TestGpuVendorsFromContainers(t *testing.T) {
	initContainers := []corev1.Container{
		{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
			"nvidia.com/gpu": resource.MustParse("1"),
		}}},
	}
	containers := []corev1.Container{
		{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
			"nvidia.com/gpu": resource.MustParse("1"),
			"amd.com/gpu":    resource.MustParse("1"),
		}}},
	}

	vendors := gpuVendorsFromContainers(initContainers, containers)
	assert.Equal(t, []string{"amd", "nvidia"}, vendors)
}
