// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sclient "k8s.io/client-go/kubernetes"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// podLister lists pods for a workload by namespace and label selector.
type podLister interface {
	listPods(ctx context.Context, namespace string, selector labels.Selector) ([]*workloadmeta.KubernetesPod, error)
}

// kubePodLister implements podLister using the Kubernetes API.
// The k8s API server filters by label selector at the etcd level, avoiding O(all_pods) in-process scans.
type kubePodLister struct {
	client k8sclient.Interface
}

func newKubePodLister(client k8sclient.Interface) podLister {
	return &kubePodLister{client: client}
}

func (l *kubePodLister) listPods(ctx context.Context, namespace string, sel labels.Selector) ([]*workloadmeta.KubernetesPod, error) {
	list, err := l.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: sel.String(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*workloadmeta.KubernetesPod, 0, len(list.Items))
	for i := range list.Items {
		pod := &list.Items[i]
		owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
		for _, ref := range pod.OwnerReferences {
			owners = append(owners, workloadmeta.KubernetesPodOwner{Kind: ref.Kind, Name: ref.Name})
		}
		result = append(result, &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   string(pod.UID),
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Labels:    pod.Labels,
			},
			Owners: owners,
			Phase:  string(pod.Status.Phase),
		})
	}
	return result, nil
}

// fetchRequest is an item in the podFetcher work queue.
type fetchRequest struct {
	w        workload
	selector labels.Selector
}

// podFetcher enqueues and processes pod backfill requests for newly opted-in workloads.
// When a workload is first added to the config store, it fetches the workload's existing
// pods by label selector and feeds them into the tracker so the rebalancer has accurate state.
type podFetcher struct {
	queue   chan fetchRequest
	lister  podLister
	tracker *podTracker
}

func newPodFetcher(lister podLister, tracker *podTracker) *podFetcher {
	return &podFetcher{
		queue:   make(chan fetchRequest, 64),
		lister:  lister,
		tracker: tracker,
	}
}

// enqueue schedules a pod backfill for w. Non-blocking: if the queue is full the
// request is dropped (a subsequent config-store event will re-enqueue it).
func (f *podFetcher) enqueue(w workload, selector labels.Selector) {
	select {
	case f.queue <- fetchRequest{w: w, selector: selector}:
	default:
		log.Warnf("spot pod fetcher queue full, dropping backfill for %s", w)
	}
}

// start processes fetch requests until ctx is cancelled.
func (f *podFetcher) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-f.queue:
			pods, err := f.lister.listPods(ctx, req.w.Namespace, req.selector)
			if err != nil {
				log.Errorf("spot pod fetcher: listing pods for %s: %v", req.w, err)
				continue
			}
			for _, pod := range pods {
				f.tracker.addedOrUpdated(pod)
			}
			log.Debugf("spot pod fetcher: backfilled %d pods for %s", len(pods), req.w)
		}
	}
}
