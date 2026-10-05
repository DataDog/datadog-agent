// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func (w *Webhook) injectAgentSidecar(ctx context.Context, pod *corev1.Pod, ns string, dc dynamic.Interface, api kubernetes.Interface, dry *bool) (bool, error) {
	if pod == nil {
		return w.planAgentSidecar(ctx, nil, ns, dc, api, dry)
	}
	return patchtest.Run(pod, ns, dc, func(s *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
		return w.planAgentSidecar(ctx, s, ns, dc, api, dry)
	})
}
func applyProviderOverrides(pod *corev1.Pod, provider string) (bool, error) {
	return patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return planProviderOverrides(s, provider)
	})
}
func applyProfileOverrides(container *corev1.Container, profiles []ProfileOverride) (bool, error) {
	if container == nil {
		return false, errors.New("can't apply profile overrides to nil containers")
	}
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{*container}}}
	changed, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return planProfileOverrides(s, patch.ContainerID{Kind: patch.RegularContainers, Name: container.Name}, profiles)
	})
	if err == nil {
		*container = pod.Spec.Containers[0]
	}
	return changed, err
}
func attachVolume(pod *corev1.Pod, volume corev1.Volume) error {
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, planAttachVolume(s, volume)
	})
	return err
}

// Existing override assertions exercise the admission writers, not typed replacements.
func withEnvOverrides(c *corev1.Container, envs ...corev1.EnvVar) (bool, error) {
	if c == nil {
		return false, errors.New("can't apply environment overrides to nil container")
	}
	return applyProfileOverrides(c, []ProfileOverride{{EnvVars: envs}})
}
func withResourceLimits(c *corev1.Container, resources corev1.ResourceRequirements) error {
	if c == nil {
		return errors.New("can't apply resource requirements overrides to nil container")
	}
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{*c}}}
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, s.ConfigureResources(patch.ContainerID{Kind: patch.RegularContainers, Name: c.Name}, resources)
	})
	if err == nil {
		*c = pod.Spec.Containers[0]
	}
	return err
}
func withSecurityContextOverrides(c *corev1.Container, sc *corev1.SecurityContext) (bool, error) {
	if c == nil {
		return false, errors.New("can't apply security context overrides to nil container")
	}
	if sc == nil {
		return false, nil
	}
	return applyProfileOverrides(c, []ProfileOverride{{SecurityContext: sc}})
}
