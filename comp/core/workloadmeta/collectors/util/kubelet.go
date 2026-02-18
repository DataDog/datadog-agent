// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package util

// The function exposed in this file is only used by the Kubelet workloadmeta
// collector. It lives here instead of in the collector code (which is under an
// "internal" directory) so we can also use it in kubelet check tests (see
// pkg/collector/corechecks/containers/kubelet/provider/pod/provider_test.go).
// Those tests use files with kubelet responses. To test easily, we need to
// parse those responses and fill workloadmeta. Without this function, we would
// need a real kubelet collector, which makes tests async and harder to run.

import (
	stdErrors "errors"
	"slices"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/third_party/golang/expansion"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const dockerImageIDPrefix = "docker-pullable://"

// ParseKubeletPods parses a list of kubelet pods and returns a list of workloadmeta events
func ParseKubeletPods(pods []*kubelet.Pod, collectEphemeralContainers bool, store workloadmeta.Component) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}

	for _, pod := range pods {
		podMeta := pod.Metadata
		if podMeta.UID == "" {
			log.Debugf("pod has no UID. meta: %+v", podMeta)
			continue
		}

		// Validate allocation size.
		// Limits hardcoded here are huge enough to never be hit.
		if len(pod.Spec.Containers) > 10000 ||
			len(pod.Spec.InitContainers) > 10000 || len(pod.Spec.EphemeralContainers) > 10000 {
			log.Errorf("pod %s has a crazy number of containers: %d, init containers: %d or ephemeral containers: %d. Skipping it!",
				podMeta.UID, len(pod.Spec.Containers), len(pod.Spec.InitContainers), len(pod.Spec.EphemeralContainers))
			continue
		}

		podID := workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podMeta.UID,
		}

		podInitContainers, initContainerEvents := parsePodContainers(
			pod,
			pod.Spec.InitContainers,
			pod.Status.InitContainers,
			&podID,
		)

		podContainers, containerEvents := parsePodContainers(
			pod,
			pod.Spec.Containers,
			pod.Status.Containers,
			&podID,
		)

		var podEphemeralContainers []workloadmeta.OrchestratorContainer
		var ephemeralContainerEvents []workloadmeta.CollectorEvent
		var ephemeralContainerStatuses []workloadmeta.KubernetesContainerStatus
		if collectEphemeralContainers {
			podEphemeralContainers, ephemeralContainerEvents = parsePodContainers(
				pod,
				pod.Spec.EphemeralContainers,
				pod.Status.EphemeralContainers,
				&podID,
			)

			ephemeralContainerStatuses = convertContainerStatuses(pod.Status.EphemeralContainers)
		}

		GPUVendors := getGPUVendorsFromContainers(initContainerEvents, containerEvents)

		podOwners := pod.Owners()
		owners := make([]workloadmeta.KubernetesPodOwner, 0, len(podOwners))
		for _, o := range podOwners {
			owners = append(owners, workloadmeta.KubernetesPodOwner{
				Kind:       o.Kind,
				Name:       o.Name,
				ID:         o.ID,
				Controller: o.Controller,
			})
		}

		PodSecurityContext := extractPodSecurityContext(&pod.Spec)
		RuntimeClassName := extractPodRuntimeClassName(&pod.Spec)

		var startTime *time.Time
		if !pod.Status.StartTime.IsZero() {
			startTime = &pod.Status.StartTime
		}

		// Lookup cached namespace entity from kubemetadata collector
		// Namespace information is not available in the kubelet API, however,
		// in the Agent namespace data is tightly coupled to the Pod entity and it's tags.
		var namespaceLabels, namespaceAnnotations map[string]string
		nsEntityID := GenerateKubeMetadataEntityID("", "namespaces", "", podMeta.Namespace)
		nsEntity, err := store.GetKubernetesMetadata(nsEntityID)
		if err == nil && nsEntity != nil {
			namespaceLabels = nsEntity.Labels
			namespaceAnnotations = nsEntity.Annotations
		}

		entity := &workloadmeta.KubernetesPod{
			EntityID: podID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:        podMeta.Name,
				Namespace:   podMeta.Namespace,
				Annotations: podMeta.Annotations,
				Labels:      podMeta.Labels,
			},
			Owners:                     owners,
			PersistentVolumeClaimNames: pod.GetPersistentVolumeClaimNames(),
			InitContainers:             podInitContainers,
			Containers:                 podContainers,
			EphemeralContainers:        podEphemeralContainers,
			Ready:                      kubelet.IsPodReady(pod),
			Phase:                      pod.Status.Phase,
			IP:                         pod.Status.PodIP,
			PriorityClass:              pod.Spec.PriorityClassName,
			QOSClass:                   pod.Status.QOSClass,
			GPUVendorList:              GPUVendors,
			RuntimeClass:               RuntimeClassName,
			NamespaceLabels:            namespaceLabels,
			NamespaceAnnotations:       namespaceAnnotations,
			SecurityContext:            PodSecurityContext,
			CreationTimestamp:          podMeta.CreationTimestamp,
			DeletionTimestamp:          podMeta.DeletionTimestamp,
			StartTime:                  startTime,
			NodeName:                   pod.Spec.NodeName,
			HostIP:                     pod.Status.HostIP,
			HostNetwork:                pod.Spec.HostNetwork,
			InitContainerStatuses:      convertContainerStatuses(pod.Status.InitContainers),
			ContainerStatuses:          convertContainerStatuses(pod.Status.Containers),
			EphemeralContainerStatuses: ephemeralContainerStatuses,
			Conditions:                 convertConditions(pod.Status.Conditions),
			Volumes:                    convertVolumes(pod.Spec.Volumes),
			Tolerations:                convertTolerations(pod.Spec.Tolerations),
			Reason:                     pod.Status.Reason,
		}

		events = append(events, initContainerEvents...)
		events = append(events, containerEvents...)
		events = append(events, ephemeralContainerEvents...)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	return events
}

func parsePodContainers(
	pod *kubelet.Pod,
	containerSpecs []kubelet.ContainerSpec,
	containerStatuses []kubelet.ContainerStatus,
	parent *workloadmeta.EntityID,
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(containerStatuses))
	events := make([]workloadmeta.CollectorEvent, 0, len(containerStatuses))

	for _, container := range containerStatuses {
		if container.ID == "" {
			// A container without an ID has not been created by
			// the runtime yet, so we ignore them until it's
			// detected again.
			continue
		}

		var containerSecurityContext *workloadmeta.ContainerSecurityContext
		var readinessProbe *workloadmeta.ContainerProbe
		var env map[string]string
		var ports []workloadmeta.ContainerPort
		var resources workloadmeta.ContainerResources
		var resizePolicy workloadmeta.ContainerResizePolicy

		// When running on docker, the image ID contains a prefix that's not
		// included in other runtimes. Remove it for consistency.
		// See https://github.com/kubernetes/kubernetes/issues/95968
		imageID := strings.TrimPrefix(container.ImageID, dockerImageIDPrefix)

		image, err := workloadmeta.NewContainerImage(imageID, container.Image)
		if err != nil {
			if stdErrors.Is(err, pkgcontainersimage.ErrImageIsSha256) {
				// try the resolved image ID if the image name in the container
				// status is a SHA256. this seems to happen sometimes when
				// pinning the image to a SHA256
				image, err = workloadmeta.NewContainerImage(imageID, imageID)
			}

			if err != nil {
				log.Debugf("cannot split image name %q nor %q: %s", container.Image, imageID, err)
			}
		}

		runtime, containerID := containers.SplitEntityName(container.ID)
		podContainer := workloadmeta.OrchestratorContainer{
			ID:   containerID,
			Name: container.Name,
		}

		containerSpec := findContainerSpec(container.Name, containerSpecs)
		if containerSpec != nil {
			env = extractEnvFromSpec(containerSpec.Env)
			resources = extractResources(containerSpec)
			resizePolicy = extractResizePolicy(containerSpec)

			podContainer.Image, err = workloadmeta.NewContainerImage(imageID, containerSpec.Image)
			if err != nil {
				log.Debugf("cannot split image name %q: %s", containerSpec.Image, err)
			}

			// Prefer the image from the spec over the status for the container entity
			image = podContainer.Image

			podContainer.Image.ID = imageID
			containerSecurityContext = extractContainerSecurityContext(containerSpec)
			readinessProbe = extractReadinessProbe(containerSpec)
			ports = make([]workloadmeta.ContainerPort, 0, len(containerSpec.Ports))
			for _, port := range containerSpec.Ports {
				ports = append(ports, workloadmeta.ContainerPort{
					Name:     port.Name,
					Port:     port.ContainerPort,
					Protocol: port.Protocol,
				})
			}
		} else {
			log.Debugf("cannot find spec for container %q", container.Name)
		}

		var allocatedResources []workloadmeta.ContainerAllocatedResource
		for _, resource := range container.ResolvedAllocatedResources {
			allocatedResources = append(allocatedResources, workloadmeta.ContainerAllocatedResource{
				Name: resource.Name,
				ID:   resource.ID,
			})
		}

		containerState := workloadmeta.ContainerState{}
		if st := container.State.Running; st != nil {
			containerState.Running = true
			containerState.Status = workloadmeta.ContainerStatusRunning
			containerState.StartedAt = st.StartedAt
			containerState.CreatedAt = st.StartedAt // CreatedAt not available
		} else if st := container.State.Terminated; st != nil {
			containerState.Running = false
			containerState.Status = workloadmeta.ContainerStatusStopped
			containerState.CreatedAt = st.StartedAt
			containerState.StartedAt = st.StartedAt
			containerState.FinishedAt = st.FinishedAt
		}

		// Kubelet considers containers without probe to be ready
		if container.Ready {
			containerState.Health = workloadmeta.ContainerHealthHealthy
		} else {
			containerState.Health = workloadmeta.ContainerHealthUnhealthy
		}

		podContainers = append(podContainers, podContainer)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.Name,
					Labels: map[string]string{
						kubernetes.CriContainerNamespaceLabel: pod.Metadata.Namespace,
					},
				},
				Image:                      image,
				EnvVars:                    env,
				SecurityContext:            containerSecurityContext,
				ReadinessProbe:             readinessProbe,
				Ports:                      ports,
				Runtime:                    workloadmeta.ContainerRuntime(runtime),
				State:                      containerState,
				Owner:                      parent,
				Resources:                  resources,
				ResizePolicy:               resizePolicy,
				ResolvedAllocatedResources: allocatedResources,
			},
		})
	}

	return podContainers, events
}

func getGPUVendorsFromContainers(initContainerEvents, containerEvents []workloadmeta.CollectorEvent) []string {
	gpuUniqueTypes := make(map[string]bool)
	for _, event := range append(initContainerEvents, containerEvents...) {
		container := event.Entity.(*workloadmeta.Container)
		for _, GPUVendor := range container.Resources.GPUVendorList {
			gpuUniqueTypes[GPUVendor] = true
		}
	}

	GPUVendors := make([]string, 0, len(gpuUniqueTypes))
	for GPUVendor := range gpuUniqueTypes {
		GPUVendors = append(GPUVendors, GPUVendor)
	}

	return GPUVendors
}

func extractPodRuntimeClassName(spec *kubelet.Spec) string {
	if spec.RuntimeClassName == nil {
		return ""
	}
	return *spec.RuntimeClassName
}

func extractPodSecurityContext(spec *kubelet.Spec) *workloadmeta.PodSecurityContext {
	if spec.SecurityContext == nil {
		return nil
	}

	return &workloadmeta.PodSecurityContext{
		RunAsUser:  spec.SecurityContext.RunAsUser,
		RunAsGroup: spec.SecurityContext.RunAsGroup,
		FsGroup:    spec.SecurityContext.FsGroup,
	}
}

func extractContainerSecurityContext(spec *kubelet.ContainerSpec) *workloadmeta.ContainerSecurityContext {
	if spec.SecurityContext == nil {
		return nil
	}

	var caps *workloadmeta.Capabilities
	if spec.SecurityContext.Capabilities != nil {
		caps = &workloadmeta.Capabilities{
			Add:  spec.SecurityContext.Capabilities.Add,
			Drop: spec.SecurityContext.Capabilities.Drop,
		}
	}

	privileged := false
	if spec.SecurityContext.Privileged != nil {
		privileged = *spec.SecurityContext.Privileged
	}

	var seccompProfile *workloadmeta.SeccompProfile
	if spec.SecurityContext.SeccompProfile != nil {
		localhostProfile := ""
		if spec.SecurityContext.SeccompProfile.LocalhostProfile != nil {
			localhostProfile = *spec.SecurityContext.SeccompProfile.LocalhostProfile
		}

		spType := workloadmeta.SeccompProfileType(spec.SecurityContext.SeccompProfile.Type)

		seccompProfile = &workloadmeta.SeccompProfile{
			Type:             spType,
			LocalhostProfile: localhostProfile,
		}
	}

	return &workloadmeta.ContainerSecurityContext{
		Capabilities:   caps,
		Privileged:     privileged,
		SeccompProfile: seccompProfile,
	}
}

func extractReadinessProbe(spec *kubelet.ContainerSpec) *workloadmeta.ContainerProbe {
	if spec.ReadinessProbe == nil {
		return nil
	}

	return &workloadmeta.ContainerProbe{
		InitialDelaySeconds: int32(spec.ReadinessProbe.InitialDelaySeconds),
	}
}

func extractEnvFromSpec(envSpec []kubelet.EnvVar) map[string]string {
	// filter out env vars that have external sources (eg. ConfigMap, Secret, etc.)
	envSpec = slices.DeleteFunc(envSpec, func(v kubelet.EnvVar) bool {
		return v.ValueFrom != nil
	})

	env := make(map[string]string)
	mappingFunc := expansion.MappingFuncFor(env)

	// TODO: Implement support of environment variables set from ConfigMap,
	// Secret, DownwardAPI.
	// See https://github.com/kubernetes/kubernetes/blob/d20fd4088476ec39c5ae2151b8fffaf0f4834418/pkg/kubelet/kubelet_pods.go#L566
	// for the complete environment variable resolution process that is
	// done by the kubelet.

	for _, e := range envSpec {
		if !containers.EnvVarFilterFromConfig().IsIncluded(e.Name) {
			continue
		}

		ok := true
		runtimeVal := e.Value
		if runtimeVal != "" {
			runtimeVal, ok = expansion.Expand(runtimeVal, mappingFunc)
		}

		// Ignore environment variables that failed to expand
		// This occurs when the env var references another env var
		// that has its value sourced from an external source
		// (eg. ConfigMap, Secret, DownwardAPI)
		if !ok {
			continue
		}

		env[e.Name] = runtimeVal
	}

	return env
}

func extractResources(spec *kubelet.ContainerSpec) workloadmeta.ContainerResources {
	resources := workloadmeta.ContainerResources{}

	if spec.Resources == nil {
		// Ephemeral containers do not have resources defined
		return resources
	}

	if cpuReq, found := spec.Resources.Requests[kubelet.ResourceCPU]; found {
		resources.CPURequest = kubernetes.FormatCPURequests(cpuReq)
	}

	if memoryReq, found := spec.Resources.Requests[kubelet.ResourceMemory]; found {
		resources.MemoryRequest = kubernetes.FormatMemoryRequests(memoryReq)
	}

	if cpuLimit, found := spec.Resources.Limits[kubelet.ResourceCPU]; found {
		resources.CPULimit = kubernetes.FormatCPURequests(cpuLimit)
	}

	if memoryLimit, found := spec.Resources.Limits[kubelet.ResourceMemory]; found {
		resources.MemoryLimit = kubernetes.FormatMemoryRequests(memoryLimit)
	}

	// Check if the CPU Requested is a whole core or cores
	if cpuReq, found := spec.Resources.Requests[kubelet.ResourceCPU]; found {
		if cpuReq.MilliValue()%1000 == 0 {
			resources.RequestedWholeCores = pointer.Ptr(true)
		}
	}

	// extract GPU resource info from the possible GPU sources
	uniqueGPUVendor := make(map[string]struct{})
	for resourceName := range spec.Resources.Requests {
		gpuName, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName))
		if found {
			uniqueGPUVendor[gpuName] = struct{}{}
		}
	}

	gpuVendorList := make([]string, 0, len(uniqueGPUVendor))
	for GPUVendor := range uniqueGPUVendor {
		gpuVendorList = append(gpuVendorList, GPUVendor)
	}
	resources.GPUVendorList = gpuVendorList

	resources.RawRequests = make(map[string]string)
	for resourceName, quantity := range spec.Resources.Requests {
		resources.RawRequests[string(resourceName)] = quantity.String()
	}

	resources.RawLimits = make(map[string]string)
	for resourceName, quantity := range spec.Resources.Limits {
		resources.RawLimits[string(resourceName)] = quantity.String()
	}

	return resources
}

func extractResizePolicy(spec *kubelet.ContainerSpec) workloadmeta.ContainerResizePolicy {
	policy := workloadmeta.ContainerResizePolicy{}

	if spec.ResizePolicy == nil {
		return policy
	}

	for _, rule := range spec.ResizePolicy {
		if rule.ResourceName == kubelet.ResourceCPU {
			policy.CPURestartPolicy = string(rule.RestartPolicy)
		}

		if rule.ResourceName == kubelet.ResourceMemory {
			policy.MemoryRestartPolicy = string(rule.RestartPolicy)
		}
	}

	return policy
}

func findContainerSpec(name string, specs []kubelet.ContainerSpec) *kubelet.ContainerSpec {
	for _, spec := range specs {
		if spec.Name == name {
			return &spec
		}
	}

	return nil
}

func convertVolumes(volumes []kubelet.VolumeSpec) []workloadmeta.KubernetesPodVolume {
	if volumes == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodVolume, len(volumes))

	for i, volume := range volumes {
		result[i] = workloadmeta.KubernetesPodVolume{
			Name: volume.Name,
		}

		if volume.PersistentVolumeClaim != nil {
			result[i].PersistentVolumeClaim = &workloadmeta.KubernetesPersistentVolumeClaim{
				ClaimName: volume.PersistentVolumeClaim.ClaimName,
				ReadOnly:  volume.PersistentVolumeClaim.ReadOnly,
			}
		}

		if volume.Ephemeral != nil && volume.Ephemeral.VolumeClaimTemplate != nil {
			result[i].Ephemeral = &workloadmeta.KubernetesEphemeralVolume{
				Name:        volume.Ephemeral.VolumeClaimTemplate.Metadata.Name,
				UID:         volume.Ephemeral.VolumeClaimTemplate.Metadata.UID,
				Annotations: volume.Ephemeral.VolumeClaimTemplate.Metadata.Annotations,
				Labels:      volume.Ephemeral.VolumeClaimTemplate.Metadata.Labels,
			}
		}
	}

	return result
}

func convertTolerations(tolerations []kubelet.Toleration) []workloadmeta.KubernetesPodToleration {
	if tolerations == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodToleration, len(tolerations))

	for i, toleration := range tolerations {
		result[i] = workloadmeta.KubernetesPodToleration{
			Key:               toleration.Key,
			Operator:          toleration.Operator,
			Value:             toleration.Value,
			Effect:            toleration.Effect,
			TolerationSeconds: toleration.TolerationSeconds,
		}
	}

	return result
}

func convertConditions(conditions []kubelet.Conditions) []workloadmeta.KubernetesPodCondition {
	if conditions == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodCondition, len(conditions))

	for i, condition := range conditions {
		result[i] = workloadmeta.KubernetesPodCondition{
			Type:   condition.Type,
			Status: condition.Status,
			Reason: condition.Reason,
		}
	}

	return result
}

func convertContainerStatuses(containerStatuses []kubelet.ContainerStatus) []workloadmeta.KubernetesContainerStatus {
	if containerStatuses == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesContainerStatus, len(containerStatuses))

	for i, status := range containerStatuses {
		result[i] = workloadmeta.KubernetesContainerStatus{
			ContainerID:          status.ID,
			Name:                 status.Name,
			Image:                status.Image,
			ImageID:              status.ImageID,
			Ready:                status.Ready,
			RestartCount:         int32(status.RestartCount),
			State:                convertContainerState(status.State),
			LastTerminationState: convertContainerState(status.LastState),
		}
	}

	return result
}

func convertContainerState(state kubelet.ContainerState) workloadmeta.KubernetesContainerState {
	result := workloadmeta.KubernetesContainerState{}

	if state.Waiting != nil {
		result.Waiting = &workloadmeta.KubernetesContainerStateWaiting{
			Reason: state.Waiting.Reason,
		}
	}

	if state.Running != nil {
		result.Running = &workloadmeta.KubernetesContainerStateRunning{
			StartedAt: state.Running.StartedAt,
		}
	}

	if state.Terminated != nil {
		result.Terminated = &workloadmeta.KubernetesContainerStateTerminated{
			ExitCode:   state.Terminated.ExitCode,
			StartedAt:  state.Terminated.StartedAt,
			FinishedAt: state.Terminated.FinishedAt,
			Reason:     state.Terminated.Reason,
		}
	}

	return result
}
