// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func (m *TargetMutator) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	if pod == nil {
		return m.PlanPod(nil, ns, dc)
	}
	return patchtest.Run(pod, ns, dc, m.PlanPod)
}
func (w *Webhook) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return patchtest.Run(pod, ns, dc, w.PlanPod)
}
func mutateContainerForTest(m containerMutator, c *corev1.Container) error {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{*c}}}
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, m.planContainer(s, patch.ContainerID{Kind: patch.RegularContainers, Name: c.Name})
	})
	if err == nil {
		*c = pod.Spec.Containers[0]
	}
	return err
}
func mutatePodForTest(m podMutator, pod *corev1.Pod) error {
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) { return true, m.planPod(s) })
	return err
}
func injectLibConfig(pod *corev1.Pod, lang language) error {
	return mutatePodForTest((&libConfigInjector{}).podMutator(lang), pod)
}
