// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	workloadWatcherStoreID autoscaling.SenderID = "prof-w"

	refreshPeriod = 30 * time.Second
	noResync      = 0
)

var namespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}

// workloadInformer holds the informer state for a single workload resource.
type workloadInformer struct {
	gvkr   GroupVersionKindResource
	lister cache.GenericLister
	synced cache.InformerSynced
}

// nsWorkloadWatcher holds per-namespace informer state for watching all
// workloads in a namespace that has the profile label.
type nsWorkloadWatcher struct {
	profileName string
	factory     dynamicinformer.DynamicSharedInformerFactory
	informers   []workloadInformer
	cancel      context.CancelFunc
}

func (nw *nsWorkloadWatcher) hasSynced() bool {
	for _, inf := range nw.informers {
		if !inf.synced() {
			return false
		}
	}
	return true
}

// WorkloadWatcher watches Kubernetes workloads for the profile label and
// updates PodAutoscalerProfileInternal entries in the profile store with the
// discovered workload references. It also watches Namespaces with the profile
// label and dynamically creates per-namespace informers to discover all
// workloads in those namespaces. Workload-level labels take precedence over
// namespace-level labels. The ProfileController reacts to those updates and
// manages the DPA store entries.
type WorkloadWatcher struct {
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal]
	isLeader     func() bool

	dynamicClient     dynamic.Interface
	workloadResources []GroupVersionKindResource

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	informers       []workloadInformer

	nsLister   cache.GenericLister
	nsSynced   cache.InformerSynced
	nsWatchers map[string]*nsWorkloadWatcher

	refreshPeriod time.Duration

	hasSynced atomic.Bool
}

// NewWorkloadWatcher creates a new WorkloadWatcher. It creates a single
// label-filtered informer factory that watches both workloads and namespaces
// with the profile label.
func NewWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
	isLeader func() bool,
	dynamicClient dynamic.Interface,
	workloadResources []GroupVersionKindResource,
) *WorkloadWatcher {
	filteredFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		noResync,
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = model.ProfileLabelKey
		},
	)

	nsInformer := filteredFactory.ForResource(namespaceGVR)

	w := &WorkloadWatcher{
		profileStore:      profileStore,
		isLeader:          isLeader,
		dynamicClient:     dynamicClient,
		workloadResources: workloadResources,
		informerFactory:   filteredFactory,
		nsLister:          nsInformer.Lister(),
		nsSynced:          nsInformer.Informer().HasSynced,
		nsWatchers:        make(map[string]*nsWorkloadWatcher),
		refreshPeriod:     refreshPeriod,
	}

	for _, resource := range workloadResources {
		inf := filteredFactory.ForResource(resource.GroupVersionResource)
		w.informers = append(w.informers, workloadInformer{
			gvkr:   resource,
			lister: inf.Lister(),
			synced: inf.Informer().HasSynced,
		})
	}

	return w
}

// HasSynced returns true once informer caches are synced and the initial
// reconcile has completed, meaning the profile store has been populated with
// workload references at least once.
func (w *WorkloadWatcher) HasSynced() bool {
	return w.hasSynced.Load()
}

// Run starts the WorkloadWatcher. It blocks until ctx is cancelled.
func (w *WorkloadWatcher) Run(ctx context.Context) {
	log.Info("Starting workload and namespace watcher")
	w.informerFactory.Start(ctx.Done())

	syncFuncs := make([]cache.InformerSynced, 0, len(w.informers)+1)
	for _, inf := range w.informers {
		syncFuncs = append(syncFuncs, inf.synced)
	}
	syncFuncs = append(syncFuncs, w.nsSynced)
	if !cache.WaitForCacheSync(ctx.Done(), syncFuncs...) {
		log.Error("Failed to sync informer caches")
		return
	}

	log.Info("Initial reconciliation starting")
	w.reconcile()
	w.hasSynced.Store(true)
	log.Info("Initial reconciliation done")

	ticker := time.NewTicker(w.refreshPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if w.isLeader() {
				w.reconcile()
			}
		case <-ctx.Done():
			log.Info("Stopping workload watcher")
			return
		}
	}
}

// reconcile scans all labeled workloads and namespace-level workloads, groups
// them by profile name, and updates the profile store with the discovered
// workload references. Workload-level labels take precedence over namespace-level.
func (w *WorkloadWatcher) reconcile() {
	workloadRefs := make(map[string][]model.NamespacedObjectReference)
	for _, inf := range w.informers {
		w.scanWorkloads(inf.gvkr, inf.lister, workloadRefs)
	}

	w.syncNsWatchers()
	w.scanNsWorkloads(workloadRefs)

	w.profileStore.Update(func(pi model.PodAutoscalerProfileInternal) (model.PodAutoscalerProfileInternal, bool) {
		changed := pi.UpdateWorkloads(workloadRefs[pi.Name()])
		return pi, changed
	}, workloadWatcherStoreID)
}

// scanWorkloads iterates over workloads of a given kind and extracts those with
// a profile label, grouping the results by profile name.
func (w *WorkloadWatcher) scanWorkloads(
	gvkr GroupVersionKindResource,
	lister cache.GenericLister,
	workloadRefs map[string][]model.NamespacedObjectReference,
) {
	// Informers already filtered by the label selector, so we can just list all objects.
	objects, err := lister.List(labels.Everything())
	if err != nil {
		log.Debugf("Failed to list objects %s, err: %v", gvkr.GroupVersionResource.String(), err)
		return
	}

	for _, obj := range objects {
		// We're only using dynamic client
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}

		profileName, ok := unstructuredObj.GetLabels()[model.ProfileLabelKey]
		if !ok || profileName == "" {
			continue
		}

		if _, ok := w.profileStore.Get(profileName); !ok {
			log.Debugf("Profile %s referenced by workload %s/%s/%s not found, skipping", profileName, gvkr.GroupVersionResource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			continue
		}

		workloadRefs[profileName] = append(workloadRefs[profileName], model.NamespacedObjectReference{
			GroupKind: schema.GroupKind{
				Group: gvkr.GroupVersionResource.Group,
				Kind:  gvkr.Kind,
			},
			Version:   gvkr.GroupVersionResource.Version,
			Namespace: unstructuredObj.GetNamespace(),
			Name:      unstructuredObj.GetName(),
		})
	}
}

// syncNsWatchers reconciles the set of per-namespace informers with the
// current set of labeled namespaces. It creates informers for newly labeled
// namespaces and tears down informers for namespaces whose label was removed.
func (w *WorkloadWatcher) syncNsWatchers() {
	// namespace.name -> profile.name
	desired := make(map[string]string)

	nsList, err := w.nsLister.List(labels.Everything())
	if err != nil {
		log.Debugf("Failed to list labeled namespaces: %v", err)
		return
	}
	for _, nsObj := range nsList {
		nsUnstructured, ok := nsObj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		profileName, ok := nsUnstructured.GetLabels()[model.ProfileLabelKey]
		if !ok || profileName == "" {
			continue
		}
		if _, ok := w.profileStore.Get(profileName); !ok {
			log.Debugf("Profile %s referenced by namespace %s not found, skipping", profileName, nsUnstructured.GetName())
			continue
		}
		desired[nsUnstructured.GetName()] = profileName
	}

	// remove informers for namespaces that are no longer labeled and update existing informers profiles
	for nsName, nw := range w.nsWatchers {
		newProfile, ok := desired[nsName]
		if !ok {
			log.Infof("Stopping namespace workload watcher for %s", nsName)
			nw.cancel()
			delete(w.nsWatchers, nsName)
			continue
		}

		if nw.profileName != newProfile {
			log.Infof("Changing profile from %v to %v for namespace %v", nw.profileName, newProfile, nsName)
			nw.profileName = newProfile
		}
	}

	// start informers for new labeled namespaces
	for nsName, profileName := range desired {
		if _, exists := w.nsWatchers[nsName]; exists {
			continue
		}
		w.startNsWatcher(nsName, profileName)
	}
}

func (w *WorkloadWatcher) startNsWatcher(nsName, profileName string) {
	ctx, cancel := context.WithCancel(context.Background())

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		w.dynamicClient,
		noResync,
		nsName,
		nil,
	)

	nw := &nsWorkloadWatcher{
		profileName: profileName,
		factory:     factory,
		cancel:      cancel,
	}

	for _, gvkr := range w.workloadResources {
		inf := factory.ForResource(gvkr.GroupVersionResource)
		nw.informers = append(nw.informers, workloadInformer{
			gvkr:   gvkr,
			lister: inf.Lister(),
			synced: inf.Informer().HasSynced,
		})
	}

	factory.Start(ctx.Done())
	log.Infof("Started namespace workload watcher for %s (profile: %s)", nsName, profileName)
	w.nsWatchers[nsName] = nw
}

// scanNsWorkloads iterates over all synced per-namespace informers and collects
// workload references. Workloads that have the profile label are skipped
// because workload-level labels take precedence over namespace-level.
func (w *WorkloadWatcher) scanNsWorkloads(
	workloadRefs map[string][]model.NamespacedObjectReference,
) {
	for nsName, nw := range w.nsWatchers {
		if !nw.hasSynced() {
			log.Debugf("Namespace watcher for %s not yet synced, skipping", nsName)
			continue
		}
		for _, inf := range nw.informers {
			w.scanNsInformerWorkloads(nsName, nw.profileName, inf, workloadRefs)
		}
	}
}

func (w *WorkloadWatcher) scanNsInformerWorkloads(
	nsName, profileName string,
	inf workloadInformer,
	workloadRefs map[string][]model.NamespacedObjectReference,
) {
	objects, err := inf.lister.List(labels.Everything())
	if err != nil {
		log.Debugf("Failed to list %s in namespace %s: %v", inf.gvkr.GroupVersionResource.Resource, nsName, err)
		return
	}

	for _, obj := range objects {
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}

		labels := unstructuredObj.GetLabels()
		if _, hasLabel := labels[model.ProfileLabelKey]; hasLabel {
			continue
		}
		if labels[model.ProfileDisabledLabelKey] == "true" {
			continue
		}

		workloadRefs[profileName] = append(workloadRefs[profileName], model.NamespacedObjectReference{
			GroupKind: schema.GroupKind{
				Group: inf.gvkr.GroupVersionResource.Group,
				Kind:  inf.gvkr.Kind,
			},
			Version:   inf.gvkr.GroupVersionResource.Version,
			Namespace: nsName,
			Name:      unstructuredObj.GetName(),
		})
	}
}
