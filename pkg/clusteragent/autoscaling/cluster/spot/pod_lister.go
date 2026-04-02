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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// podLister lists pods for a workload by namespace and label selector.
type podLister interface {
	listPods(ctx context.Context, namespace string, selector string) ([]*workloadmeta.KubernetesPod, error)
}

// kubePodLister implements podLister using the Kubernetes API.
// The k8s API server filters by label selector at the etcd level, avoiding O(all_pods) in-process scans.
type kubePodLister struct {
	client k8sclient.Interface
}

func newKubePodLister(client k8sclient.Interface) podLister {
	return &kubePodLister{client: client}
}

func (l *kubePodLister) listPods(ctx context.Context, namespace string, selector string) ([]*workloadmeta.KubernetesPod, error) {
	list, err := l.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	result := make([]*workloadmeta.KubernetesPod, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, coreV1PodToWLM(&list.Items[i]))
	}
	return result, nil
}

func coreV1PodToWLM(pod *corev1.Pod) *workloadmeta.KubernetesPod {
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{Kind: ref.Kind, Name: ref.Name})
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
		Owners: owners,
		Phase:  string(pod.Status.Phase),
	}
}
