// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func applyRecommendationPlanForTest(plan *PodRecommendationPlan, pod *corev1.Pod) (bool, error) {
	if plan == nil {
		return false, nil
	}
	return patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		for _, a := range plan.Annotations {
			var err error
			if a.Value == nil {
				err = s.RemoveAnnotations(a.Key)
			} else {
				err = s.SetAnnotations(map[string]string{a.Key: *a.Value}, false)
			}
			if err != nil {
				return false, err
			}
		}
		id := func(c RecommendationContainer) patch.ContainerID {
			kind := patch.RegularContainers
			if c.Init {
				kind = patch.InitContainers
			}
			return patch.ContainerID{Kind: kind, Name: c.Name}
		}
		var edits []patch.ResourceEdit
		for _, e := range plan.Resources {
			edits = append(edits, patch.ResourceEdit{Container: id(e.Container), Limits: e.Limits, Name: e.Name, Quantity: e.Quantity})
		}
		if err := s.EditResources(edits); err != nil {
			return false, err
		}
		for _, r := range plan.Runtime {
			env := corev1.EnvVar{Name: "GOMEMLIMIT", Value: r.GOMEMLIMIT}
			c := id(r.Container)
			matches, err := s.FindEnv(c, env.Name)
			if err != nil {
				return false, err
			}
			if len(matches) > 0 {
				err = s.SetEnvOccurrence(matches[0], env)
			} else {
				_, err = s.EnsureEnvs([]patch.EnvInjection{{Container: c, Env: env}})
			}
			if err != nil {
				return false, err
			}
		}
		return plan.Changed(), nil
	})
}
func applyRecommendationsForTest(patcher PodPatcher, pod *corev1.Pod) (bool, error) {
	plan, err := patcher.PlanRecommendations(pod)
	if err != nil {
		return false, err
	}
	return applyRecommendationPlanForTest(plan, pod)
}
func patchPod(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, pod *corev1.Pod) bool {
	plan := &PodRecommendationPlan{}
	plan.recommendation(reco, pod)
	changed, err := applyRecommendationPlanForTest(plan, pod)
	if err != nil {
		panic(err)
	}
	return changed
}
func patchContainerResources(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, container *corev1.Container) bool {
	plan := &PodRecommendationPlan{}
	plan.container(reco, container, false)
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{*container}}}
	changed, err := applyRecommendationPlanForTest(plan, pod)
	if err != nil {
		panic(err)
	}
	*container = pod.Spec.Containers[0]
	return changed
}
