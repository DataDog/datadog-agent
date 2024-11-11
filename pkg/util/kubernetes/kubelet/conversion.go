// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package kubelet

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ConvertKubeletPodToK8sPod converts a Pod to a Kubernetes Pod.
// The Pod in this package is a simplification of the one in the Kubernetes
// library, so the result will not contain all the fields. That's OK, because
// this function is only called from the KSM check, so we only need to convert
// the fields that are used by the check.
func ConvertKubeletPodToK8sPod(pod *Pod) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Metadata.Name,
			UID:       types.UID(pod.Metadata.UID),
			Namespace: pod.Metadata.Namespace,
			CreationTimestamp: metav1.Time{
				Time: pod.Metadata.CreationTimestamp,
			},
			Annotations:     pod.Metadata.Annotations,
			Labels:          pod.Metadata.Labels,
			OwnerReferences: convertToK8sOwnerReferences(pod.Metadata.Owners),
		},
		Spec: corev1.PodSpec{
			HostNetwork:       pod.Spec.HostNetwork,
			NodeName:          pod.Spec.NodeName,
			InitContainers:    convertToK8sContainers(pod.Spec.InitContainers),
			Containers:        convertToK8sContainers(pod.Spec.Containers),
			Volumes:           convertToK8sVolumes(pod.Spec.Volumes),
			PriorityClassName: pod.Spec.PriorityClassName,
			SecurityContext:   convertToK8sPodSecurityContext(pod.Spec.SecurityContext),
			RuntimeClassName:  pod.Spec.RuntimeClassName,
			Tolerations:       convertToK8sPodTolerations(pod.Spec.Tolerations),
		},
		Status: corev1.PodStatus{
			Phase:                 corev1.PodPhase(pod.Status.Phase),
			HostIP:                pod.Status.HostIP,
			PodIP:                 pod.Status.PodIP,
			ContainerStatuses:     convertToK8sContainerStatuses(pod.Status.Containers),
			InitContainerStatuses: convertToK8sContainerStatuses(pod.Status.InitContainers),
			Conditions:            convertToK8sConditions(pod.Status.Conditions),
			QOSClass:              corev1.PodQOSClass(pod.Status.QOSClass),
			StartTime: &metav1.Time{
				Time: pod.Status.StartTime,
			},
			Reason: pod.Status.Reason,
		},
	}
}

func convertToK8sOwnerReferences(owners []PodOwner) []metav1.OwnerReference {
	if owners == nil {
		return nil
	}

	k8sOwnerReferences := make([]metav1.OwnerReference, len(owners))
	for i, owner := range owners {
		k8sOwnerReferences[i] = metav1.OwnerReference{
			Kind:       owner.Kind,
			Name:       owner.Name,
			Controller: owner.Controller,
		}
	}
	return k8sOwnerReferences
}

func convertToK8sContainers(containerSpecs []ContainerSpec) []corev1.Container {
	if containerSpecs == nil {
		return nil
	}

	k8sContainers := make([]corev1.Container, len(containerSpecs))
	for i, containerSpec := range containerSpecs {
		k8sContainers[i] = corev1.Container{
			Name:            containerSpec.Name,
			Image:           containerSpec.Image,
			Ports:           convertToK8sContainerPorts(containerSpec.Ports),
			ReadinessProbe:  convertToK8sProbe(containerSpec.ReadinessProbe),
			Env:             convertToK8sEnvVars(containerSpec.Env),
			SecurityContext: convertToK8sContainerSecurityContext(containerSpec.SecurityContext),
			Resources:       convertToK8sResourceRequirements(containerSpec.Resources),
		}
	}
	return k8sContainers
}

func convertToK8sVolumes(volumeSpecs []VolumeSpec) []corev1.Volume {
	if volumeSpecs == nil {
		return nil
	}

	k8sVolumes := make([]corev1.Volume, len(volumeSpecs))
	for i, volumeSpec := range volumeSpecs {
		k8sVolumes[i] = corev1.Volume{
			Name: volumeSpec.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: convertToK8sPersistentVolumeClaim(volumeSpec.PersistentVolumeClaim),
				Ephemeral:             convertToK8sEphemeralVolume(volumeSpec.Ephemeral),
			},
		}
	}
	return k8sVolumes
}

func convertToK8sPodSecurityContext(podSecurityContextSpec *PodSecurityContextSpec) *corev1.PodSecurityContext {
	if podSecurityContextSpec == nil {
		return nil
	}

	runAsUser := int64(podSecurityContextSpec.RunAsUser)
	runAsGroup := int64(podSecurityContextSpec.RunAsGroup)
	fsGroup := int64(podSecurityContextSpec.FsGroup)

	return &corev1.PodSecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
		FSGroup:    &fsGroup,
	}
}

func convertToK8sContainerPorts(containerPortSpecs []ContainerPortSpec) []corev1.ContainerPort {
	if containerPortSpecs == nil {
		return nil
	}

	k8sPorts := make([]corev1.ContainerPort, len(containerPortSpecs))
	for i, containerPortSpec := range containerPortSpecs {
		k8sPorts[i] = corev1.ContainerPort{
			ContainerPort: int32(containerPortSpec.ContainerPort),
			HostPort:      int32(containerPortSpec.HostPort),
			Name:          containerPortSpec.Name,
			Protocol:      corev1.Protocol(containerPortSpec.Protocol),
		}
	}
	return k8sPorts
}

func convertToK8sProbe(containerProbe *ContainerProbe) *corev1.Probe {
	if containerProbe == nil {
		return nil
	}
	return &corev1.Probe{
		InitialDelaySeconds: int32(containerProbe.InitialDelaySeconds),
	}
}

func convertToK8sEnvVars(envVars []EnvVar) []corev1.EnvVar {
	if envVars == nil {
		return nil
	}

	k8sEnvVars := make([]corev1.EnvVar, len(envVars))
	for i, envVar := range envVars {
		k8sEnvVars[i] = corev1.EnvVar{
			Name:  envVar.Name,
			Value: envVar.Value,
		}
	}
	return k8sEnvVars
}

func convertToK8sContainerSecurityContext(containerSecurityContextSpec *ContainerSecurityContextSpec) *corev1.SecurityContext {
	if containerSecurityContextSpec == nil {
		return nil
	}
	return &corev1.SecurityContext{
		Capabilities:   convertToK8sCapabilities(containerSecurityContextSpec.Capabilities),
		Privileged:     containerSecurityContextSpec.Privileged,
		SeccompProfile: convertToK8sSeccompProfile(containerSecurityContextSpec.SeccompProfile),
	}
}

func convertToK8sCapabilities(capabilities *CapabilitiesSpec) *corev1.Capabilities {
	if capabilities == nil {
		return nil
	}

	res := &corev1.Capabilities{}

	for _, addCapability := range capabilities.Add {
		res.Add = append(res.Add, corev1.Capability(addCapability))
	}

	for _, dropCapability := range capabilities.Drop {
		res.Drop = append(res.Drop, corev1.Capability(dropCapability))
	}

	return res
}

func convertToK8sSeccompProfile(seccompProfileSpec *SeccompProfileSpec) *corev1.SeccompProfile {
	if seccompProfileSpec == nil {
		return nil
	}
	return &corev1.SeccompProfile{
		Type:             corev1.SeccompProfileType(seccompProfileSpec.Type),
		LocalhostProfile: seccompProfileSpec.LocalhostProfile,
	}
}

func convertToK8sResourceRequirements(containerResourcesSpec *ContainerResourcesSpec) corev1.ResourceRequirements {
	if containerResourcesSpec == nil {
		return corev1.ResourceRequirements{}
	}
	return corev1.ResourceRequirements{
		Requests: convertToK8sResourceList(containerResourcesSpec.Requests),
		Limits:   convertToK8sResourceList(containerResourcesSpec.Limits),
	}
}

func convertToK8sResourceList(resourceList ResourceList) corev1.ResourceList {
	k8sResourceList := make(corev1.ResourceList)
	for k, v := range resourceList {
		k8sResourceList[corev1.ResourceName(k)] = v
	}
	return k8sResourceList
}

func convertToK8sPersistentVolumeClaim(persistentVolumeClaimSpec *PersistentVolumeClaimSpec) *corev1.PersistentVolumeClaimVolumeSource {
	if persistentVolumeClaimSpec == nil {
		return nil
	}
	return &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: persistentVolumeClaimSpec.ClaimName,
		ReadOnly:  persistentVolumeClaimSpec.ReadOnly,
	}
}

func convertToK8sEphemeralVolume(ephemeralSpec *EphemeralSpec) *corev1.EphemeralVolumeSource {
	if ephemeralSpec == nil {
		return nil
	}
	return &corev1.EphemeralVolumeSource{
		VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ephemeralSpec.VolumeClaimTemplate.Metadata.Name,
				UID:         types.UID(ephemeralSpec.VolumeClaimTemplate.Metadata.UID),
				Annotations: ephemeralSpec.VolumeClaimTemplate.Metadata.Annotations,
				Labels:      ephemeralSpec.VolumeClaimTemplate.Metadata.Labels,
			},
		},
	}
}

func convertToK8sContainerStatuses(containerStatuses []ContainerStatus) []corev1.ContainerStatus {
	if containerStatuses == nil {
		return nil
	}

	k8sStatuses := make([]corev1.ContainerStatus, len(containerStatuses))
	for i, containerStatus := range containerStatuses {
		k8sStatuses[i] = corev1.ContainerStatus{
			Name:                 containerStatus.Name,
			Image:                containerStatus.Image,
			ImageID:              containerStatus.ImageID,
			ContainerID:          containerStatus.ID,
			Ready:                containerStatus.Ready,
			RestartCount:         int32(containerStatus.RestartCount),
			State:                convertToK8sContainerState(containerStatus.State),
			LastTerminationState: convertToK8sContainerState(containerStatus.LastState),
		}
	}
	return k8sStatuses
}

func convertToK8sContainerState(containerState ContainerState) corev1.ContainerState {
	return corev1.ContainerState{
		Waiting:    convertToK8sContainerStateWaiting(containerState.Waiting),
		Running:    convertToK8sContainerStateRunning(containerState.Running),
		Terminated: convertToK8sContainerStateTerminated(containerState.Terminated),
	}
}

func convertToK8sContainerStateWaiting(containerStateWaiting *ContainerStateWaiting) *corev1.ContainerStateWaiting {
	if containerStateWaiting == nil {
		return nil
	}
	return &corev1.ContainerStateWaiting{
		Reason: containerStateWaiting.Reason,
	}
}

func convertToK8sContainerStateRunning(containerStateRunning *ContainerStateRunning) *corev1.ContainerStateRunning {
	if containerStateRunning == nil {
		return nil
	}
	return &corev1.ContainerStateRunning{
		StartedAt: metav1.Time{Time: containerStateRunning.StartedAt},
	}
}

func convertToK8sContainerStateTerminated(containerStateTerminated *ContainerStateTerminated) *corev1.ContainerStateTerminated {
	if containerStateTerminated == nil {
		return nil
	}
	return &corev1.ContainerStateTerminated{
		ExitCode:   containerStateTerminated.ExitCode,
		StartedAt:  metav1.Time{Time: containerStateTerminated.StartedAt},
		FinishedAt: metav1.Time{Time: containerStateTerminated.FinishedAt},
		Reason:     containerStateTerminated.Reason,
	}
}

func convertToK8sConditions(conditions []Conditions) []corev1.PodCondition {
	if conditions == nil {
		return nil
	}

	k8sConditions := make([]corev1.PodCondition, len(conditions))
	for i, condition := range conditions {
		k8sConditions[i] = corev1.PodCondition{
			Type:   corev1.PodConditionType(condition.Type),
			Status: corev1.ConditionStatus(condition.Status),
		}
	}
	return k8sConditions
}

func convertToK8sPodTolerations(tolerations []Toleration) []corev1.Toleration {
	if tolerations == nil {
		return nil
	}

	k8sTolerations := make([]corev1.Toleration, len(tolerations))
	for i, toleration := range tolerations {
		k8sTolerations[i] = corev1.Toleration{
			Key:               toleration.Key,
			Operator:          corev1.TolerationOperator(toleration.Operator),
			Value:             toleration.Value,
			Effect:            corev1.TaintEffect(toleration.Effect),
			TolerationSeconds: toleration.TolerationSeconds,
		}
	}
	return k8sTolerations
}
