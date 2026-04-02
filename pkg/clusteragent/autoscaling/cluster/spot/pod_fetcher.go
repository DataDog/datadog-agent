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
	"k8s.io/client-go/util/workqueue"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// podLister lists pods for a workload by namespace and label selector.
type podLister interface {
	listPods(ctx context.Context, namespace string, selector string) ([]*workloadmeta.KubernetesPod, error)
}

// podFetcher processes pod fetch requests.
// It fetches the workload's existing pods by label selector and
// feeds them into the tracker so it has accurate state.
type podFetcher struct {
	queue   workqueue.TypedRateLimitingInterface[fetchRequest]
	lister  podLister
	tracker *podTracker
}

// fetchRequest is an item in the podFetcher work queue.
type fetchRequest struct {
	workload workload
	selector string
}

func newPodFetcher(lister podLister, tracker *podTracker) *podFetcher {
	return &podFetcher{
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedItemBasedRateLimiter[fetchRequest](),
			workqueue.TypedRateLimitingQueueConfig[fetchRequest]{Name: "spot-pod-fetcher"},
		),
		lister:  lister,
		tracker: tracker,
	}
}

// enqueue schedules a pod fetch request for workload.
// Requests are deduplicated by the work queue.
func (f *podFetcher) enqueue(workload workload, selector string) {
	f.queue.Add(fetchRequest{workload: workload, selector: selector})
}

// start processes fetch requests until ctx is cancelled.
func (f *podFetcher) start(ctx context.Context) {
	stop := context.AfterFunc(ctx, f.queue.ShutDown)
	defer stop()

	for f.processNext(ctx) {
	}
}

func (f *podFetcher) processNext(ctx context.Context) bool {
	req, shutdown := f.queue.Get()
	if shutdown {
		return false
	}
	defer f.queue.Done(req)

	pods, err := f.lister.listPods(ctx, req.workload.Namespace, req.selector)
	if err != nil {
		log.Errorf("spot pod fetcher: listing pods for %s: %v", req.workload, err)
		f.queue.AddRateLimited(req)
		return true
	}

	f.queue.Forget(req)
	for _, pod := range pods {
		f.tracker.addedOrUpdated(pod)
	}
	log.Debugf("spot pod fetcher: fetched %d pods for %s", len(pods), req.workload)
	return true
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
