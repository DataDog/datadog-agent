// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func (i *Mutator) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return patchtest.Run(pod, ns, dc, i.PlanPod)
}
func injectTagsFromLabels(labels map[string]string, pod *corev1.Pod) (bool, bool) {
	var found bool
	injected, _ := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		var changed bool
		var err error
		found, changed, err = planTagsFromLabels(labels, s)
		return changed, err
	})
	return found, injected
}
func (w *Webhook) inject(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return patchtest.Run(pod, ns, dc, w.mutator.PlanPod)
}
