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

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NamespaceLister abstracts the retrieval of namespace metadata so that
// the WorkloadWatcher can consume either a real WorkloadMetaStore or a
// fake implementation in tests.
type NamespaceLister interface {
	// ListNamespaces returns namespace -> [label -> value]
	ListNamespaces() map[string]map[string]string
}

type wmsNamespaceLister struct {
	wlm wmdef.Component
}

var _ NamespaceLister = (*wmsNamespaceLister)(nil)

func (l *wmsNamespaceLister) ListNamespaces() map[string]map[string]string {
	nsList := l.wlm.ListKubernetesMetadata(func(m *wmdef.KubernetesMetadata) bool {
		return wmdef.IsNamespaceMetadata(m)
	})
	result := make(map[string]map[string]string, len(nsList))
	for _, ns := range nsList {
		result[ns.Name] = ns.Labels
	}
	return result
}

const (
	workloadWatcherStoreID autoscaling.SenderID = "prof-w"

	refreshPeriod = 30 * time.Second
	noResync      = 0
)

// workloadInformer holds the informer state for a single workload resource.
type workloadInformer struct {
	gvkr   GroupVersionKindResource
	lister cache.GenericLister
	synced cache.InformerSynced
}

// WorkloadWatcher watches Kubernetes workloads for the profile label and
// updates PodAutoscalerProfileInternal entries in the profile store with the
// discovered workload references. It also watches Namespaces with the profile
// label and uses dynamic LIST calls to discover all workloads in those
// namespaces. Workload-level labels take precedence over namespace-level
// labels. The ProfileController reacts to those updates and manages the DPA
// store entries.
type WorkloadWatcher struct {
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal]
	isLeader     func() bool

	dynamicClient     dynamic.Interface
	workloadResources []GroupVersionKindResource

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	informers       []workloadInformer

	nsLister NamespaceLister

	refreshPeriod time.Duration

	hasSynced atomic.Bool
}

// NewWorkloadWatcher creates a new WorkloadWatcher. It creates a
// label-filtered informer factory that watches workloads with the profile
// label, and uses the WorkloadMetaStore to discover labeled namespaces.
func NewWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
	isLeader func() bool,
	dynamicClient dynamic.Interface,
	workloadResources []GroupVersionKindResource,
	wlm wmdef.Component,
) *WorkloadWatcher {
	filteredFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		noResync,
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = model.ProfileLabelKey
		},
	)

	w := &WorkloadWatcher{
		profileStore:      profileStore,
		isLeader:          isLeader,
		dynamicClient:     dynamicClient,
		workloadResources: workloadResources,
		informerFactory:   filteredFactory,
		nsLister:          &wmsNamespaceLister{wlm: wlm},
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
	log.Info("Starting workload watcher")
	w.informerFactory.Start(ctx.Done())

	syncFuncs := make([]cache.InformerSynced, 0, len(w.informers))
	for _, inf := range w.informers {
		syncFuncs = append(syncFuncs, inf.synced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), syncFuncs...) {
		log.Error("Failed to sync informer caches")
		return
	}

	log.Info("Initial reconciliation starting")
	w.reconcile(ctx)
	w.hasSynced.Store(true)
	log.Info("Initial reconciliation done")

	ticker := time.NewTicker(w.refreshPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if w.isLeader() {
				w.reconcile(ctx)
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
func (w *WorkloadWatcher) reconcile(ctx context.Context) {
	workloadRefs := make(map[string][]model.NamespacedObjectReference)
	for _, inf := range w.informers {
		w.scanWorkloads(inf.gvkr, inf.lister, workloadRefs)
	}

	w.scanNsWorkloads(ctx, workloadRefs)

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

// scanNsWorkloads discovers labeled namespaces and uses dynamic LIST calls to
// collect all workloads in those namespaces. Workloads that already have the
// profile label are skipped (workload-level takes precedence).
func (w *WorkloadWatcher) scanNsWorkloads(
	ctx context.Context,
	workloadRefs map[string][]model.NamespacedObjectReference,
) {
	for nsName, nsLabels := range w.nsLister.ListNamespaces() {
		profileName := nsLabels[model.ProfileLabelKey]
		if profileName == "" {
			continue
		}
		pi, ok := w.profileStore.Get(profileName)
		if !ok {
			log.Debugf("Profile %s referenced by namespace %s not found, skipping", profileName, nsName)
			continue
		}

		for _, gvkr := range w.workloadResources {
			w.listNsWorkloads(ctx, pi, nsName, profileName, gvkr, workloadRefs)
		}
	}
}

func (w *WorkloadWatcher) listNsWorkloads(
	ctx context.Context,
	pi model.PodAutoscalerProfileInternal,
	nsName, profileName string,
	gvkr GroupVersionKindResource,
	workloadRefs map[string][]model.NamespacedObjectReference,
) {
	result, err := w.dynamicClient.Resource(gvkr.GroupVersionResource).Namespace(nsName).List(ctx, metav1.ListOptions{})
	if err != nil {
		// On a transient API failure (timeout, 429, network stall) we must not
		// treat previously discovered workloads as removed. Each reconcile
		// rebuilds workloadRefs from scratch, so if we simply skip this
		// namespace/resource the profile will end up with an empty ref list,
		// causing the syncer to delete the generated DPAs even though the
		// workloads still exist. To avoid this destructive action we populate
		// the refs with that the profile already holds for this
		// namespace/resource kind.
		log.Warnf("Failed to list %s in namespace %s: %v; preserving previous refs", gvkr.GroupVersionResource.Resource, nsName, err)
		for _, ref := range pi.Workloads() {
			if ref.Namespace == nsName && ref.Group == gvkr.GroupVersionResource.Group && ref.Kind == gvkr.Kind {
				workloadRefs[profileName] = append(workloadRefs[profileName], ref)
			}
		}
		return
	}

	for i := range result.Items {
		obj := &result.Items[i]

		objLabels := obj.GetLabels()
		if _, hasLabel := objLabels[model.ProfileLabelKey]; hasLabel {
			continue
		}
		if objLabels[model.ProfileEnabledLabelKey] == "false" {
			continue
		}

		workloadRefs[profileName] = append(workloadRefs[profileName], model.NamespacedObjectReference{
			GroupKind: schema.GroupKind{
				Group: gvkr.GroupVersionResource.Group,
				Kind:  gvkr.Kind,
			},
			Version:   gvkr.GroupVersionResource.Version,
			Namespace: nsName,
			Name:      obj.GetName(),
		})
	}
}
