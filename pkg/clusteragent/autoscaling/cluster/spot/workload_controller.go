// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type workloadResource struct {
	gvr  schema.GroupVersionResource
	kind string
}

var spotWorkloadResources = []workloadResource{
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, kubernetes.DeploymentKind},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, kubernetes.StatefulSetKind},
}

// workloadController watches spot-enabled workloads and keeps the podTracker and
// spotConfigStore up to date.
type workloadController struct {
	defaultConfig workloadSpotConfig
	store         workloadConfigStore
	podLister     podLister
	tracker       *podTracker

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	listers         map[string]cache.GenericLister
	hasSynced       []cache.InformerSynced
	synced          chan struct{}

	queue workqueue.TypedRateLimitingInterface[workload]
}

func newWorkloadController(dynamicClient dynamic.Interface, defaultConfig workloadSpotConfig, store workloadConfigStore, lister podLister, tracker *podTracker) *workloadController {
	c := &workloadController{
		defaultConfig: defaultConfig,
		store:         store,
		podLister:     lister,
		tracker:       tracker,
		synced:        make(chan struct{}),
		listers:       make(map[string]cache.GenericLister),
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedItemBasedRateLimiter[workload](),
			workqueue.TypedRateLimitingQueueConfig[workload]{Name: "spot-workload-controller"},
		),
	}

	c.informerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		0, // no resync
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = SpotEnabledLabelKey + "=" + SpotEnabledLabelValue
		},
	)

	for _, r := range spotWorkloadResources {
		kind := r.kind
		inf := c.informerFactory.ForResource(r.gvr)
		if reg, err := inf.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				c.onUpdated(kind, obj)
			},
			UpdateFunc: func(_, obj any) {
				c.onUpdated(kind, obj)
			},
			DeleteFunc: func(obj any) {
				c.onDeleted(kind, obj)
			},
		}); err != nil {
			log.Errorf("Failed to add event handler for %s: %v", r.gvr.Resource, err)
		} else {
			c.hasSynced = append(c.hasSynced, reg.HasSynced)
			c.listers[kind] = inf.Lister()
		}
	}

	return c
}

// onUpdated handles workload add/update events: updates the config store and enqueues for tracker reconciliation.
func (c *workloadController) onUpdated(kind string, obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	key := workload{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}

	if isSpotEnabled(u) {
		cfg := c.defaultConfig
		overrideFromAnnotations(&cfg, u.GetAnnotations())

		c.store.setConfig(key, cfg)

		log.Debugf("Spot workload config updated %s: %#v", key, cfg)
	} else {
		c.store.deleteConfig(key)

		log.Debugf("Spot workload config deleted %s", key)
	}

	c.queue.Add(key)
}

// onDeleted handles workload delete events: removes the config store entry and enqueues for tracker reconciliation.
func (c *workloadController) onDeleted(kind string, obj any) {
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = d.Obj
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	key := workload{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}
	c.store.deleteConfig(key)

	log.Debugf("Spot workload deleted %s", key)

	c.queue.Add(key)
}

// start starts the informers, waits for cache sync, then processes the work queue until ctx is cancelled.
func (c *workloadController) start(ctx context.Context) {
	stop := context.AfterFunc(ctx, c.queue.ShutDown)
	defer stop()

	c.informerFactory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), c.hasSynced...) {
		log.Error("Spot workload controller: failed to sync informer caches")
		close(c.synced)
		return
	}
	log.Info("Spot workload controller synced")
	close(c.synced)

	for c.processNext(ctx) {
	}
}

// waitSynced blocks until the controller has completed its initial cache sync.
func (c *workloadController) waitSynced() {
	<-c.synced
}

func (c *workloadController) processNext(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	if err := c.processItem(ctx, key); err != nil {
		log.Errorf("Spot workload controller: processing %s: %v", key, err)
		c.queue.AddRateLimited(key)
		return true
	}
	c.queue.Forget(key)
	return true
}

// processItem reconciles the tracker state for a single workload.
// If the workload opts in to spot scheduling it lists its pods and feeds them into the tracker.
// If the workload is opted out or no longer exists it calls tracker.untrack.
func (c *workloadController) processItem(ctx context.Context, key workload) error {
	lister, ok := c.listers[key.Kind]
	if !ok {
		return fmt.Errorf("no lister for kind %s", key.Kind)
	}

	obj, err := lister.ByNamespace(key.Namespace).Get(key.Name)
	if k8serrors.IsNotFound(err) {
		c.tracker.untrack(key)
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting %s: %w", key, err)
	}

	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}

	if !isSpotEnabled(u) {
		c.tracker.untrack(key)
		return nil
	}

	selector, ok := getPodSelector(key.Kind, u.Object)
	if !ok {
		return nil
	}

	pods, err := c.podLister.listPods(ctx, key.Namespace, selector)
	if err != nil {
		return fmt.Errorf("listing pods for %s: %w", key, err)
	}

	for _, pod := range pods {
		c.tracker.addedOrUpdated(pod)
	}
	log.Debugf("Spot workload controller: fetched %d pods for %s", len(pods), key)

	return nil
}

// getPodSelector returns the label selector string for a workload object based on its kind.
// Returns the selector and true if the selector can be determined, or empty string and false otherwise.
func getPodSelector(kind string, obj map[string]any) (string, bool) {
	switch kind {
	case kubernetes.DeploymentKind, kubernetes.StatefulSetKind:
		matchLabels, _, _ := unstructured.NestedStringMap(obj, "spec", "selector", "matchLabels")
		if len(matchLabels) > 0 {
			return labels.SelectorFromSet(labels.Set(matchLabels)).String(), true
		}
	}
	return "", false
}

// isSpotEnabled reports whether the unstructured object has the spot-enabled label set.
func isSpotEnabled(u *unstructured.Unstructured) bool {
	return u.GetLabels()[SpotEnabledLabelKey] == SpotEnabledLabelValue
}
