// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package mutate

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/wI2L/jsondiff"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

type mutateFunc func(*corev1.Pod, string, dynamic.Interface) error

// mutate handles mutating pods and encoding and decoding admission
// requests and responses for the public mutate functions
func mutate(rawPod []byte, ns string, m mutateFunc, dc dynamic.Interface) ([]byte, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(rawPod, &pod); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %v", err)
	}

	if err := m(&pod, ns, dc); err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the mutated Pod object: %v", err)
	}

	patch, err := jsondiff.CompareJSON(rawPod, bytes) // TODO: Try to generate the patch at the mutateFunc
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

// envIndex returns the index of env var in an env var list
// returns -1 if not found
func envIndex(envs []corev1.EnvVar, name string) int {
	for i := range envs {
		if envs[i].Name == name {
			return i
		}
	}

	return -1
}

// injectEnv injects an env var into a pod template if it doesn't exist
func injectEnv(pod *corev1.Pod, env corev1.EnvVar) bool {
	injected := false
	podStr := podString(pod)
	log.Debugf("Injecting env var '%s' into pod %s", env.Name, podStr)
	for i, ctr := range pod.Spec.Containers {
		if contains(ctr.Env, env.Name) {
			log.Debugf("Ignoring container '%s' in pod %s: env var '%s' already exist", ctr.Name, podStr, env.Name)
			continue
		}
		// prepend rather than append so that our new vars precede container vars in the final list, so that they
		// can be referenced in other env vars downstream.  (see:  Kubernetes dependent environment variables.)
		pod.Spec.Containers[i].Env = append([]corev1.EnvVar{env}, pod.Spec.Containers[i].Env...)
		injected = true
	}
	for i, ctr := range pod.Spec.InitContainers {
		if contains(ctr.Env, env.Name) {
			log.Debugf("Ignoring init container '%s' in pod %s: env var '%s' already exist", ctr.Name, podStr, env.Name)
			continue
		}
		// prepend rather than append so that our new vars precede container vars in the final list, so that they
		// can be referenced in other env vars downstream.  (see:  Kubernetes dependent environment variables.)
		pod.Spec.InitContainers[i].Env = append([]corev1.EnvVar{env}, pod.Spec.InitContainers[i].Env...)
		injected = true
	}
	return injected
}

// injectVolume injects a volume into a pod template if it doesn't exist
func injectVolume(pod *corev1.Pod, volume corev1.Volume, volumeMount corev1.VolumeMount) bool {
	podStr := podString(pod)
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

// podString returns a string that helps identify the pod
func podString(pod *corev1.Pod) string {
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
