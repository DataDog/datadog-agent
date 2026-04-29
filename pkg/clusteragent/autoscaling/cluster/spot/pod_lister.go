// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// podLister lists pods for a workload by namespace and label selector.
type podLister interface {
	listPods(ctx context.Context, namespace string, selector string) ([]*workloadmeta.KubernetesPod, error)
}

// wlmPodLister implements podLister using the in-process workloadmeta store.
// This avoids API server calls since we already watch all pods via workloadmeta.
type wlmPodLister struct {
	wlm workloadmeta.Component
}

func newWLMPodLister(wlm workloadmeta.Component) podLister {
	return &wlmPodLister{wlm: wlm}
}

func (l *wlmPodLister) listPods(_ context.Context, namespace string, selector string) ([]*workloadmeta.KubernetesPod, error) {
	sel, err := labels.Parse(selector)
	if err != nil {
		return nil, err
	}
	var result []*workloadmeta.KubernetesPod
	for _, pod := range l.wlm.ListKubernetesPods() {
		if pod.Namespace == namespace && sel.Matches(labels.Set(pod.Labels)) {
			result = append(result, pod)
		}
	}
	return result, nil
}

// coreV1PodToWLM converts a corev1.Pod to a workloadmeta.KubernetesPod.
func coreV1PodToWLM(pod *corev1.Pod) *workloadmeta.KubernetesPod {
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		gv, _ := schema.ParseGroupVersion(ref.APIVersion)
		owners = append(owners, workloadmeta.KubernetesPodOwner{Kind: ref.Kind, Name: ref.Name, Group: gv.Group})
	}
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Namespace:   pod.Namespace,
			Name:        pod.Name,
			Labels:      maps.Clone(pod.Labels),
			Annotations: maps.Clone(pod.Annotations),
		},
		Owners:            owners,
		Phase:             string(pod.Status.Phase),
		CreationTimestamp: pod.CreationTimestamp.Time,
		NodeName:          pod.Spec.NodeName,
	}
}
