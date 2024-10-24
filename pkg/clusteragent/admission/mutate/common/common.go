// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides functions used by several mutating webhooks
package common

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wI2L/jsondiff"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// K8sAutoscalerSafeToEvictVolumesAnnotation is the annotation used by the
// Kubernetes cluster-autoscaler to mark a volume as safe to evict
const K8sAutoscalerSafeToEvictVolumesAnnotation = "cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes"

// MutationFunc is a function that mutates a pod
type MutationFunc func(pod *corev1.Pod, ns string, cl dynamic.Interface) (bool, error)

// Mutate handles mutating pods and encoding and decoding admission
// requests and responses for the public mutate functions
func Mutate(rawPod []byte, ns string, mutationType string, m MutationFunc, dc dynamic.Interface) ([]byte, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(rawPod, &pod); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %v", err)
	}

	injected, err := m(&pod, ns, dc)
	if err != nil {
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, strconv.FormatBool(false), err.Error())
		return nil, fmt.Errorf("failed to mutate pod: %v", err)
	}

	metrics.MutationAttempts.Inc(mutationType, metrics.StatusSuccess, strconv.FormatBool(injected), "")

	bytes, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the mutated Pod object: %v", err)
	}

	patch, err := jsondiff.CompareJSON(rawPod, bytes) // TODO: Try to generate the patch at the MutationFunc
	if err != nil {
		return nil, fmt.Errorf("failed to prepare the JSON patch: %v", err)
	}

	return json.Marshal(patch)
}

// contains returns whether EnvVar slice contains an env var with a given name
func contains(envs []corev1.EnvVar, name string) bool {
	for _, env := range envs {
		if env.Name == name {
			return true
		}
	}
	return false
}

// InjectEnv injects an env var into a pod template.
func InjectEnv(pod *corev1.Pod, env corev1.EnvVar) (injected bool) {
	log.Debugf("Injecting env var '%s' into pod %s", env.Name, PodString(pod))

	return InjectDynamicEnv(pod, func(_ *corev1.Container, _ bool) (corev1.EnvVar, error) {
		return env, nil
	})
}

// injectEnvInContainer injects an env var into a container if it doesn't exist.
func injectEnvInContainer(container *corev1.Container, env corev1.EnvVar) (injected bool) {
	if contains(container.Env, env.Name) {
		log.Debugf("Ignoring container '%s': env var '%s' already exist", container.Name, env.Name)
		return
	}

	// Prepend rather than append the new variables so that they precede the previous ones in the final list,
	// allowing them to be referenced in other environment variables downstream.
	// (See: https://kubernetes.io/docs/tasks/inject-data-application/define-interdependent-environment-variables)
	container.Env = append([]corev1.EnvVar{env}, container.Env...)
	return true
}

// BuildEnvVarFunc is a function that builds a dynamic env var.
type BuildEnvVarFunc func(container *corev1.Container, init bool) (corev1.EnvVar, error)

// InjectDynamicEnv injects a dynamic env var into a pod template.
func InjectDynamicEnv(pod *corev1.Pod, fn BuildEnvVarFunc) (injected bool) {
	log.Debugf("Injecting env var into pod %s", PodString(pod))
	injected = injectDynamicEnvInContainers(pod.Spec.Containers, fn, false)
	injected = injectDynamicEnvInContainers(pod.Spec.InitContainers, fn, true) || injected
	return
}

// injectDynamicEnvInContainers injects a dynamic env var into a list of containers.
func injectDynamicEnvInContainers(containers []corev1.Container, fn BuildEnvVarFunc, init bool) (injected bool) {
	for i := range containers {
		env, err := fn(&containers[i], init)
		if err != nil {
			log.Errorf("Error building env var: %v", err)
			continue
		}
		injected = injectEnvInContainer(&containers[i], env) || injected
	}

	return injected
}

// InjectVolume injects a volume into a pod template if it doesn't exist
func InjectVolume(pod *corev1.Pod, volume corev1.Volume, volumeMount corev1.VolumeMount) bool {
	podStr := PodString(pod)
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == volume.Name {
			log.Debugf("Ignoring pod %q: volume %q already exists", podStr, vol.Name)
			return false
		}
	}

	shouldInject := false
	for i, container := range pod.Spec.Containers {
		if containsVolumeMount(container.VolumeMounts, volumeMount) {
			// Ensure volume mount name and path uniqueness
			log.Debugf("Ignoring container %q in pod %q: a volume mount with name %q or path %q already exists", container.Name, podStr, volumeMount.Name, volumeMount.MountPath)
			continue
		}
		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, volumeMount)
		shouldInject = true
	}
	for i, container := range pod.Spec.InitContainers {
		if containsVolumeMount(container.VolumeMounts, volumeMount) {
			// Ensure volume mount name and path uniqueness
			log.Debugf("Ignoring init container %q in pod %q: a volume mount with name %q or path %q already exists", container.Name, podStr, volumeMount.Name, volumeMount.MountPath)
			continue
		}
		pod.Spec.InitContainers[i].VolumeMounts = append(pod.Spec.InitContainers[i].VolumeMounts, volumeMount)
		shouldInject = true
	}

	if shouldInject {
		pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
	}

	return shouldInject
}

// PodString returns a string that helps identify the pod
func PodString(pod *corev1.Pod) string {
	if pod.GetNamespace() == "" || pod.GetName() == "" {
		return fmt.Sprintf("with generate name %s", pod.GetGenerateName())
	}
	return fmt.Sprintf("%s/%s", pod.GetNamespace(), pod.GetName())
}

// containsVolumeMount returns whether a list of volume mounts contains
// at least one volume mount with a given name or mount path
func containsVolumeMount(volumeMounts []corev1.VolumeMount, element corev1.VolumeMount) bool {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == element.Name {
			return true
		}
		if volumeMount.MountPath == element.MountPath {
			return true
		}
	}
	return false
}

// ContainerRegistry gets the container registry config using the specified
// config option, and falls back to the default container registry if no
// webhook-specific container registry is set.
func ContainerRegistry(datadogConfig config.Component, specificConfigOpt string) string {
	if datadogConfig.IsSet(specificConfigOpt) {
		return datadogConfig.GetString(specificConfigOpt)
	}

	return datadogConfig.GetString("admission_controller.container_registry")
}

// MarkVolumeAsSafeToEvictForAutoscaler adds the Kubernetes cluster-autoscaler
// annotation to the given pod, marking the specified local volume as safe to
// evict. This annotation allows the cluster-autoscaler to evict pods with the
// local volume mounted, enabling the node to scale down if necessary.
// This function will not add the volume to the annotation if it is already
// there.
// Ref: https://github.com/kubernetes/autoscaler/blob/cluster-autoscaler-release-1.31/cluster-autoscaler/FAQ.md#what-types-of-pods-can-prevent-ca-from-removing-a-node
func MarkVolumeAsSafeToEvictForAutoscaler(pod *corev1.Pod, volumeNameToAdd string) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	currentVolumes := pod.Annotations[K8sAutoscalerSafeToEvictVolumesAnnotation]
	var volumeList []string
	if currentVolumes != "" {
		volumeList = strings.Split(currentVolumes, ",")
	}

	if slices.Contains(volumeList, volumeNameToAdd) {
		return // Volume already in the list, no need to add
	}

	volumeList = append(volumeList, volumeNameToAdd)
	pod.Annotations[K8sAutoscalerSafeToEvictVolumesAnnotation] = strings.Join(volumeList, ",")
}
