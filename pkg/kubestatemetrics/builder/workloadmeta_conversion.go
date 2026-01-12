// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// convertWorkloadmetaPodToK8sPod converts a workloadmeta KubernetesPod to a Kubernetes Pod.
// The workloadmeta KubernetesPod is a simplification of the one in the
// Kubernetes library, so the result will not contain all the fields. That's OK,
// because this function is only called from the KSM check, so we only need to
// convert the fields that are used by the check.
func convertWorkloadmetaPodToK8sPod(pod *workloadmeta.KubernetesPod, wmeta workloadmeta.Component) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.EntityMeta.Name,
			UID:       types.UID(pod.EntityID.ID),
			Namespace: pod.EntityMeta.Namespace,
			CreationTimestamp: metav1.Time{
				Time: pod.CreationTimestamp,
			},
			Annotations:     pod.EntityMeta.Annotations,
			Labels:          pod.EntityMeta.Labels,
			OwnerReferences: convertWorkloadmetaOwnerReferences(pod.Owners),
		},
		Spec: corev1.PodSpec{
			HostNetwork:       pod.HostNetwork,
			NodeName:          pod.NodeName,
			InitContainers:    convertWorkloadmetaOrchestratorContainers(pod.InitContainers, wmeta),
			Containers:        convertWorkloadmetaOrchestratorContainers(pod.Containers, wmeta),
			Volumes:           convertWorkloadmetaVolumes(pod.Volumes),
			PriorityClassName: pod.PriorityClass,
			SecurityContext:   convertWorkloadmetaPodSecurityContext(pod.SecurityContext),
			RuntimeClassName:  convertWorkloadmetaRuntimeClassName(pod.RuntimeClass),
			Tolerations:       convertWorkloadmetaTolerations(pod.Tolerations),
		},
		Status: corev1.PodStatus{
			Phase:                 corev1.PodPhase(pod.Phase),
			HostIP:                pod.HostIP,
			PodIP:                 pod.IP,
			ContainerStatuses:     convertWorkloadmetaContainerStatuses(pod.ContainerStatuses),
			InitContainerStatuses: convertWorkloadmetaContainerStatuses(pod.InitContainerStatuses),
			Conditions:            convertWorkloadmetaConditions(pod.Conditions),
			QOSClass:              corev1.PodQOSClass(pod.QOSClass),
			StartTime:             convertWorkloadmetaStartTime(pod.StartTime),
			Reason:                pod.Reason,
		},
	}
}

func convertWorkloadmetaOwnerReferences(owners []workloadmeta.KubernetesPodOwner) []metav1.OwnerReference {
	if owners == nil {
		return nil
	}

	result := make([]metav1.OwnerReference, len(owners))

	for i, owner := range owners {
		result[i] = metav1.OwnerReference{
			Kind:       owner.Kind,
			Name:       owner.Name,
			Controller: owner.Controller,
		}
	}

	return result
}

func convertWorkloadmetaOrchestratorContainers(containers []workloadmeta.OrchestratorContainer, wmeta workloadmeta.Component) []corev1.Container {
	if containers == nil {
		return nil
	}

	result := make([]corev1.Container, len(containers))

	for i, container := range containers {
		result[i] = corev1.Container{
			Name:      container.Name,
			Image:     container.Image.Name,
			Resources: convertWorkloadmetaContainerResources(container.Resources),
		}

		// Lookup full container information from workloadmeta
		containerEntity, err := wmeta.GetContainer(container.ID)
		if err != nil {
			log.Tracef("Failed to get full container entity for ID %s: %v", container.ID, err)
			continue
		}

		if containerEntity == nil {
			continue
		}

		result[i].Resources = convertWorkloadmetaContainerResources(containerEntity.Resources)
		result[i].Ports = convertWorkloadmetaContainerPorts(containerEntity.Ports)
		result[i].Env = convertWorkloadmetaEnvVars(containerEntity.EnvVars)
		result[i].SecurityContext = convertWorkloadmetaContainerSecurityContext(containerEntity.SecurityContext)
		result[i].ReadinessProbe = convertWorkloadmetaContainerProbe(containerEntity.ReadinessProbe)
	}

	return result
}

func convertWorkloadmetaContainerResources(resources workloadmeta.ContainerResources) corev1.ResourceRequirements {
	result := corev1.ResourceRequirements{}

	if resources.CPURequest != nil || resources.MemoryRequest != nil {
		result.Requests = make(corev1.ResourceList)
	}

	if resources.CPULimit != nil || resources.MemoryLimit != nil {
		result.Limits = make(corev1.ResourceList)
	}

	// CPU resources are stored as percentages (0-100*numCPU) in workloadmeta
	// but Kubernetes ResourceList expects them as core fractions (e.g., "0.1" for 10% of one core)
	if resources.CPURequest != nil {
		cpuCores := *resources.CPURequest / 100.0
		result.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpuCores*1000), resource.DecimalSI)
	}
	if resources.MemoryRequest != nil {
		result.Requests[corev1.ResourceMemory] = *resource.NewQuantity(int64(*resources.MemoryRequest), resource.BinarySI)
	}
	if resources.CPULimit != nil {
		cpuCores := *resources.CPULimit / 100.0
		result.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpuCores*1000), resource.DecimalSI)
	}
	if resources.MemoryLimit != nil {
		result.Limits[corev1.ResourceMemory] = *resource.NewQuantity(int64(*resources.MemoryLimit), resource.BinarySI)
	}

	return result
}

func convertWorkloadmetaVolumes(volumes []workloadmeta.KubernetesPodVolume) []corev1.Volume {
	if volumes == nil {
		return nil
	}

	result := make([]corev1.Volume, len(volumes))

	for i, volume := range volumes {
		result[i] = corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: convertWorkloadmetaPersistentVolumeClaim(volume.PersistentVolumeClaim),
				Ephemeral:             convertWorkloadmetaEphemeralVolume(volume.Ephemeral),
			},
		}
	}

	return result
}

func convertWorkloadmetaPersistentVolumeClaim(pvc *workloadmeta.KubernetesPersistentVolumeClaim) *corev1.PersistentVolumeClaimVolumeSource {
	if pvc == nil {
		return nil
	}

	return &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvc.ClaimName,
		ReadOnly:  pvc.ReadOnly,
	}
}

func convertWorkloadmetaEphemeralVolume(ephemeral *workloadmeta.KubernetesEphemeralVolume) *corev1.EphemeralVolumeSource {
	if ephemeral == nil {
		return nil
	}

	return &corev1.EphemeralVolumeSource{
		VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ephemeral.Name,
				UID:         types.UID(ephemeral.UID),
				Annotations: ephemeral.Annotations,
				Labels:      ephemeral.Labels,
			},
		},
	}
}

func convertWorkloadmetaPodSecurityContext(sc *workloadmeta.PodSecurityContext) *corev1.PodSecurityContext {
	if sc == nil {
		return nil
	}

	runAsUser := int64(sc.RunAsUser)
	runAsGroup := int64(sc.RunAsGroup)
	fsGroup := int64(sc.FsGroup)

	return &corev1.PodSecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
		FSGroup:    &fsGroup,
	}
}

func convertWorkloadmetaRuntimeClassName(runtimeClass string) *string {
	if runtimeClass == "" {
		return nil
	}
	return &runtimeClass
}

func convertWorkloadmetaTolerations(tolerations []workloadmeta.KubernetesPodToleration) []corev1.Toleration {
	if tolerations == nil {
		return nil
	}

	result := make([]corev1.Toleration, len(tolerations))

	for i, toleration := range tolerations {
		result[i] = corev1.Toleration{
			Key:               toleration.Key,
			Operator:          corev1.TolerationOperator(toleration.Operator),
			Value:             toleration.Value,
			Effect:            corev1.TaintEffect(toleration.Effect),
			TolerationSeconds: toleration.TolerationSeconds,
		}
	}

	return result
}

func convertWorkloadmetaContainerStatuses(containerStatuses []workloadmeta.KubernetesContainerStatus) []corev1.ContainerStatus {
	if containerStatuses == nil {
		return nil
	}

	result := make([]corev1.ContainerStatus, len(containerStatuses))

	for i, status := range containerStatuses {
		result[i] = corev1.ContainerStatus{
			Name:                 status.Name,
			Image:                status.Image,
			ImageID:              status.ImageID,
			ContainerID:          status.ContainerID,
			Ready:                status.Ready,
			RestartCount:         status.RestartCount,
			State:                convertWorkloadmetaContainerState(status.State),
			LastTerminationState: convertWorkloadmetaContainerState(status.LastTerminationState),
		}
	}

	return result
}

func convertWorkloadmetaContainerState(state workloadmeta.KubernetesContainerState) corev1.ContainerState {
	return corev1.ContainerState{
		Waiting:    convertWorkloadmetaContainerStateWaiting(state.Waiting),
		Running:    convertWorkloadmetaContainerStateRunning(state.Running),
		Terminated: convertWorkloadmetaContainerStateTerminated(state.Terminated),
	}
}

func convertWorkloadmetaContainerStateWaiting(waiting *workloadmeta.KubernetesContainerStateWaiting) *corev1.ContainerStateWaiting {
	if waiting == nil {
		return nil
	}
	return &corev1.ContainerStateWaiting{
		Reason: waiting.Reason,
	}
}

func convertWorkloadmetaContainerStateRunning(running *workloadmeta.KubernetesContainerStateRunning) *corev1.ContainerStateRunning {
	if running == nil {
		return nil
	}
	return &corev1.ContainerStateRunning{
		StartedAt: metav1.Time{Time: running.StartedAt},
	}
}

func convertWorkloadmetaContainerStateTerminated(terminated *workloadmeta.KubernetesContainerStateTerminated) *corev1.ContainerStateTerminated {
	if terminated == nil {
		return nil
	}
	return &corev1.ContainerStateTerminated{
		ExitCode:   terminated.ExitCode,
		StartedAt:  metav1.Time{Time: terminated.StartedAt},
		FinishedAt: metav1.Time{Time: terminated.FinishedAt},
		Reason:     terminated.Reason,
	}
}

func convertWorkloadmetaConditions(conditions []workloadmeta.KubernetesPodCondition) []corev1.PodCondition {
	if conditions == nil {
		return nil
	}

	result := make([]corev1.PodCondition, len(conditions))

	for i, condition := range conditions {
		result[i] = corev1.PodCondition{
			Type:   corev1.PodConditionType(condition.Type),
			Status: corev1.ConditionStatus(condition.Status),
		}
	}

	return result
}

func convertWorkloadmetaStartTime(startTime *time.Time) *metav1.Time {
	if startTime == nil {
		return nil
	}
	return &metav1.Time{Time: *startTime}
}

func convertWorkloadmetaContainerPorts(ports []workloadmeta.ContainerPort) []corev1.ContainerPort {
	if ports == nil {
		return nil
	}

	result := make([]corev1.ContainerPort, len(ports))

	for i, port := range ports {
		result[i] = corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: int32(port.Port),
			Protocol:      corev1.Protocol(port.Protocol),
			HostPort:      int32(port.HostPort),
		}
	}

	return result
}

func convertWorkloadmetaContainerProbe(probe *workloadmeta.ContainerProbe) *corev1.Probe {
	if probe == nil {
		return nil
	}

	return &corev1.Probe{
		InitialDelaySeconds: probe.InitialDelaySeconds,
	}
}

func convertWorkloadmetaEnvVars(envVars map[string]string) []corev1.EnvVar {
	if envVars == nil {
		return nil
	}

	result := make([]corev1.EnvVar, 0, len(envVars))

	for name, value := range envVars {
		result = append(result, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	return result
}

func convertWorkloadmetaContainerSecurityContext(sc *workloadmeta.ContainerSecurityContext) *corev1.SecurityContext {
	if sc == nil {
		return nil
	}

	result := &corev1.SecurityContext{
		Privileged: &sc.Privileged,
	}

	if sc.Capabilities != nil {
		addCaps := make([]corev1.Capability, len(sc.Capabilities.Add))
		for i, capability := range sc.Capabilities.Add {
			addCaps[i] = corev1.Capability(capability)
		}

		dropCaps := make([]corev1.Capability, len(sc.Capabilities.Drop))
		for i, capability := range sc.Capabilities.Drop {
			dropCaps[i] = corev1.Capability(capability)
		}

		result.Capabilities = &corev1.Capabilities{
			Add:  addCaps,
			Drop: dropCaps,
		}
	}

	if sc.SeccompProfile != nil {
		result.SeccompProfile = &corev1.SeccompProfile{
			Type:             corev1.SeccompProfileType(sc.SeccompProfile.Type),
			LocalhostProfile: &sc.SeccompProfile.LocalhostProfile,
		}
	}

	return result
}
