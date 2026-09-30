// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	stdErrors "errors"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/third_party/golang/expansion"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dockerImageIDPrefix is stripped from ImageID, mirroring the kubelet API's
// own convention for reporting pullable image references.
const dockerImageIDPrefix = "docker-pullable://"

// parsePod builds the workloadmeta events for a single pod: one
// KubernetesPod entity, plus one Container entity per (init) container that
// the runtime has already created. Only the fields consumed by the tagger
// (comp/core/tagger/collectors/workloadmeta_extract.go) are populated.
func parsePod(pod *corev1.Pod) []workloadmeta.CollectorEvent {
	podID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   string(pod.UID),
	}

	initContainers, initContainerEvents := parsePodContainers(pod.Spec.InitContainers, pod.Status.InitContainerStatuses, &podID)
	podContainers, containerEvents := parsePodContainers(pod.Spec.Containers, pod.Status.ContainerStatuses, &podID)

	events := make([]workloadmeta.CollectorEvent, 0, len(initContainerEvents)+len(containerEvents)+1)
	events = append(events, initContainerEvents...)
	events = append(events, containerEvents...)

	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		gv, _ := schema.ParseGroupVersion(o.APIVersion)
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind:       o.Kind,
			Name:       o.Name,
			ID:         string(o.UID),
			Group:      gv.Group,
			Controller: o.Controller,
		})
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	var runtimeClass string
	if pod.Spec.RuntimeClassName != nil {
		runtimeClass = *pod.Spec.RuntimeClassName
	}

	var ready bool
	var readyTimestamp *time.Time
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			ready = true
			t := condition.LastTransitionTime.Time
			readyTimestamp = &t
		}
		break
	}

	var deletionTimestamp *time.Time
	if pod.DeletionTimestamp != nil {
		t := pod.DeletionTimestamp.Time
		deletionTimestamp = &t
	}

	entity := &workloadmeta.KubernetesPod{
		EntityID: podID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: pod.Annotations,
			Labels:      pod.Labels,
		},
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		InitContainers:             initContainers,
		Containers:                 podContainers,
		Ready:                      ready,
		Phase:                      string(pod.Status.Phase),
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),
		GPUVendorList:              gpuVendorsFromContainers(pod.Spec.InitContainers, pod.Spec.Containers),
		RuntimeClass:               runtimeClass,
		NodeName:                   pod.Spec.NodeName,
		HostNetwork:                pod.Spec.HostNetwork,
		HostIP:                     pod.Status.HostIP,
		CreationTimestamp:          pod.CreationTimestamp.Time,
		DeletionTimestamp:          deletionTimestamp,
		ReadyTimestamp:             readyTimestamp,
	}

	events = append(events, workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceNodeOrchestrator,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	return events
}

// parsePodContainers builds the OrchestratorContainer references for a pod's
// entity plus the corresponding Container entity events, matching container
// statuses (from the pod's status, which carries the real container ID) with
// their container spec (from the pod's spec, which carries env vars and
// resource requirements).
func parsePodContainers(
	containerSpecs []corev1.Container,
	containerStatuses []corev1.ContainerStatus,
	parent *workloadmeta.EntityID,
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(containerStatuses))
	events := make([]workloadmeta.CollectorEvent, 0, len(containerStatuses))

	for _, status := range containerStatuses {
		if status.ContainerID == "" {
			// The runtime has not created this container yet; it will be
			// picked up on a later update once it has an ID.
			continue
		}

		runtime, containerID := containers.SplitEntityName(status.ContainerID)

		imageID := strings.TrimPrefix(status.ImageID, dockerImageIDPrefix)
		image, err := workloadmeta.NewContainerImage(imageID, status.Image)
		if err != nil {
			if stdErrors.Is(err, pkgcontainersimage.ErrImageIsSha256) {
				image, err = workloadmeta.NewContainerImage(imageID, imageID)
			}
			if err != nil {
				log.Debugf("cannot parse image name %q / %q for container %q: %s", status.Image, imageID, status.Name, err)
			}
		}

		var env map[string]string
		var resources workloadmeta.ContainerResources
		var resizePolicy workloadmeta.ContainerResizePolicy

		if spec := findContainerSpec(status.Name, containerSpecs); spec != nil {
			env = extractEnvFromSpec(spec.Env)
			resources = extractResources(spec)
			resizePolicy = extractResizePolicy(spec)

			// Prefer the image from the spec over the status: the status's
			// image can still be the previous one right after a container is
			// resized/recreated, while the spec always reflects the current
			// desired image.
			specImage, specErr := workloadmeta.NewContainerImage(imageID, spec.Image)
			if specErr != nil {
				log.Debugf("cannot parse image name %q for container %q: %s", spec.Image, status.Name, specErr)
			}
			image = specImage
		} else {
			log.Debugf("cannot find spec for container %q", status.Name)
		}

		podContainers = append(podContainers, workloadmeta.OrchestratorContainer{
			ID:           containerID,
			Name:         status.Name,
			Image:        image,
			Resources:    resources,
			ResizePolicy: resizePolicy,
		})

		containerState := workloadmeta.ContainerState{}
		switch {
		case status.State.Running != nil:
			containerState.Running = true
			containerState.Status = workloadmeta.ContainerStatusRunning
			containerState.CreatedAt = status.State.Running.StartedAt.Time
			containerState.StartedAt = status.State.Running.StartedAt.Time
		case status.State.Terminated != nil:
			containerState.Running = false
			containerState.Status = workloadmeta.ContainerStatusStopped
			containerState.CreatedAt = status.State.Terminated.StartedAt.Time
			containerState.StartedAt = status.State.Terminated.StartedAt.Time
			containerState.FinishedAt = status.State.Terminated.FinishedAt.Time
		}

		if status.Ready {
			containerState.Health = workloadmeta.ContainerHealthHealthy
		} else {
			containerState.Health = workloadmeta.ContainerHealthUnhealthy
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: status.Name,
				},
				Image:        image,
				EnvVars:      env,
				Runtime:      workloadmeta.ContainerRuntime(runtime),
				State:        containerState,
				Resources:    resources,
				ResizePolicy: resizePolicy,
				Owner:        parent,
			},
		})
	}

	return podContainers, events
}

func findContainerSpec(name string, specs []corev1.Container) *corev1.Container {
	for i := range specs {
		if specs[i].Name == name {
			return &specs[i]
		}
	}
	return nil
}

// extractEnvFromSpec resolves the static env vars declared on a container
// spec, skipping anything sourced from ValueFrom (configmap/secret/field
// refs) since those require calls this collector does not make, and
// filtering out variables not in the configured allow-list.
func extractEnvFromSpec(envSpec []corev1.EnvVar) map[string]string {
	envSpec = slices.DeleteFunc(slices.Clone(envSpec), func(v corev1.EnvVar) bool {
		return v.ValueFrom != nil
	})

	env := make(map[string]string)
	mappingFunc := expansion.MappingFuncFor(env)

	for _, e := range envSpec {
		if !containers.EnvVarFilterFromConfig().IsIncluded(e.Name) {
			continue
		}

		value := e.Value
		ok := true
		if value != "" {
			value, ok = expansion.Expand(value, mappingFunc)
		}
		if !ok {
			continue
		}

		env[e.Name] = value
	}

	return env
}

func extractResources(spec *corev1.Container) workloadmeta.ContainerResources {
	return workloadmeta.ContainerResources{
		GPUVendorList: gpuVendorsFromLimits(spec.Resources.Limits),
	}
}

func extractResizePolicy(spec *corev1.Container) workloadmeta.ContainerResizePolicy {
	var policy workloadmeta.ContainerResizePolicy

	for _, rule := range spec.ResizePolicy {
		switch rule.ResourceName {
		case corev1.ResourceCPU:
			policy.CPURestartPolicy = string(rule.RestartPolicy)
		case corev1.ResourceMemory:
			policy.MemoryRestartPolicy = string(rule.RestartPolicy)
		}
	}

	return policy
}

func gpuVendorsFromContainers(containerSpecLists ...[]corev1.Container) []string {
	unique := make(map[string]struct{})
	for _, specs := range containerSpecLists {
		for _, spec := range specs {
			for _, vendor := range gpuVendorsFromLimits(spec.Resources.Limits) {
				unique[vendor] = struct{}{}
			}
		}
	}

	vendors := make([]string, 0, len(unique))
	for vendor := range unique {
		vendors = append(vendors, vendor)
	}
	return vendors
}

func gpuVendorsFromLimits(limits corev1.ResourceList) []string {
	unique := make(map[string]struct{})
	for resourceName := range limits {
		if vendor, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName)); found {
			unique[vendor] = struct{}{}
		}
	}

	vendors := make([]string, 0, len(unique))
	for vendor := range unique {
		vendors = append(vendors, vendor)
	}
	return vendors
}
