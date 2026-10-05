// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

// These adapters preserve existing assertions while exercising the real writers.
func InjectEnv(pod *corev1.Pod, env corev1.EnvVar) bool {
	changed, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) { return PatchInjectEnv(s, env) })
	if err != nil {
		panic(err)
	}
	return changed
}
func AddAnnotation(pod *corev1.Pod, key, value string) bool {
	changed, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		p, err := s.Snapshot()
		if err != nil {
			return false, err
		}
		_, found := p.Annotations[key]
		return !found, s.SetAnnotations(map[string]string{key: value}, true)
	})
	if err != nil {
		panic(err)
	}
	return changed
}
func InjectVolume(pod *corev1.Pod, volume corev1.Volume, mount corev1.VolumeMount) (bool, bool) {
	var addedVolume, addedMount bool
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		var err error
		addedVolume, addedMount, err = PatchInjectVolume(s, volume, mount)
		return addedVolume || addedMount, err
	})
	if err != nil {
		panic(err)
	}
	return addedVolume, addedMount
}
func MarkVolumeAsSafeToEvictForAutoscaler(pod *corev1.Pod, name string) {
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, s.MarkVolumeSafeToEvict(K8sAutoscalerSafeToEvictVolumesAnnotation, name)
	})
	if err != nil {
		panic(err)
	}
}
func normalizeVolumes(volumes []corev1.Volume) ([]corev1.Volume, error) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Volumes: volumes}}
	_, err := patchtest.Run(pod, "", nil, func(session *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return false, session.NormalizeVolumes()
	})
	return pod.Spec.Volumes, err
}
