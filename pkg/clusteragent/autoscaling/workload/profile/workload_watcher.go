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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
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

var namespaceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// workloadRefsByProfile maps profile name to the list of workload references
// assigned to it. It is the return type of scanWorkloads.
type workloadRefsByProfile map[string][]model.NamespacedObjectReference

// Add merges all entries from other into w.
func (w workloadRefsByProfile) Add(other workloadRefsByProfile) {
	for profile, refs := range other {
		w[profile] = append(w[profile], refs...)
	}
}

// workloadInformer holds the informer state for a single workload resource.
type workloadInformer struct {
	gvkr   GroupVersionKindResource
	lister cache.GenericLister
	synced cache.InformerSynced
}

// WorkloadWatcher watches Kubernetes workloads and namespaces for the profile
// label and updates PodAutoscalerProfileInternal entries in the profile store
// with the discovered workload references. A single metadata-only informer
// factory covers both workload resources and namespaces. Workload-level labels
// take precedence over namespace-level labels. The ProfileController reacts to
// those updates and manages the DPA store entries.
type WorkloadWatcher struct {
	profileStore      *autoscaling.Store[model.PodAutoscalerProfileInternal]
	isLeader          func() bool
	workloadResources []GroupVersionKindResource

	informerFactory metadatainformer.SharedInformerFactory
	informers       []workloadInformer

	nsLister cache.GenericLister
	nsSynced cache.InformerSynced

	refreshPeriod time.Duration

	hasSynced atomic.Bool
}

// NewWorkloadWatcher creates a new WorkloadWatcher. It creates an unfiltered
// metadata-only informer factory that watches all workload resources and
// namespaces in the cluster.
func NewWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
	isLeader func() bool,
	metadataClient metadata.Interface,
	workloadResources []GroupVersionKindResource,
) *WorkloadWatcher {
	factory := metadatainformer.NewSharedInformerFactory(metadataClient, noResync)

	nsInformer := factory.ForResource(namespaceGVR)

	w := &WorkloadWatcher{
		profileStore:      profileStore,
		isLeader:          isLeader,
		workloadResources: workloadResources,
		informerFactory:   factory,
		nsLister:          nsInformer.Lister(),
		nsSynced:          nsInformer.Informer().HasSynced,
		refreshPeriod:     refreshPeriod,
	}

	for _, resource := range workloadResources {
		inf := factory.ForResource(resource.GroupVersionResource)
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

// reconcile scans all workloads, groups them by profile name using both
// workload-level and namespace-level labels, and updates the profile store
// with the discovered workload references.
func (w *WorkloadWatcher) reconcile() {
	labeledNamespaces := w.buildLabeledNamespaces()

	workloadRefs := make(workloadRefsByProfile)
	for _, inf := range w.informers {
		workloadRefs.Add(w.scanWorkloads(inf.gvkr, inf.lister, labeledNamespaces))
	}

	w.profileStore.Update(func(pi model.PodAutoscalerProfileInternal) (model.PodAutoscalerProfileInternal, bool) {
		changed := pi.UpdateWorkloads(workloadRefs[pi.Name()])
		return pi, changed
	}, workloadWatcherStoreID)
}

// buildLabeledNamespaces returns a map of namespace name → profile name for
// all namespaces that have the profile label and whose profile exists in the
// profile store.
func (w *WorkloadWatcher) buildLabeledNamespaces() map[string]string {
	labeledNamespaces := make(map[string]string)

	nsList, err := w.nsLister.List(labels.Everything())
	if err != nil {
		log.Debugf("Failed to list namespaces: %v", err)
		return labeledNamespaces
	}

	for _, obj := range nsList {
		nsMeta, ok := obj.(*metav1.PartialObjectMetadata)
		if !ok {
			continue
		}
		profileName := nsMeta.Labels[model.ProfileLabelKey]
		if profileName == "" {
			continue
		}
		if _, ok := w.profileStore.Get(profileName); !ok {
			log.Debugf("Profile %s referenced by namespace %s not found, skipping", profileName, nsMeta.Name)
			continue
		}
		labeledNamespaces[nsMeta.Name] = profileName
	}
	return labeledNamespaces
}

// scanWorkloads iterates over all workloads of a given kind, resolves the
// profile for each one, and returns the resulting workloadRefsByProfile.
// A workload with the profile label is assigned directly (workload-level).
// Otherwise, if the workload's namespace carries the profile label, it is
// assigned via namespace-level (unless opted out with profile=excluded).
// Workload-level labels take precedence.
func (w *WorkloadWatcher) scanWorkloads(
	gvkr GroupVersionKindResource,
	lister cache.GenericLister,
	labeledNamespaces map[string]string,
) workloadRefsByProfile {
	result := make(workloadRefsByProfile)

	objects, err := lister.List(labels.Everything())
	if err != nil {
		log.Debugf("Failed to list objects %s, err: %v", gvkr.GroupVersionResource.String(), err)
		return result
	}

	for _, obj := range objects {
		objMeta, ok := obj.(*metav1.PartialObjectMetadata)
		if !ok {
			continue
		}

		profileName, ok := w.resolveProfile(gvkr, objMeta, labeledNamespaces)
		if !ok {
			continue
		}

		result[profileName] = append(result[profileName], model.NamespacedObjectReference{
			GroupKind: schema.GroupKind{
				Group: gvkr.GroupVersionResource.Group,
				Kind:  gvkr.Kind,
			},
			Version:   gvkr.GroupVersionResource.Version,
			Namespace: objMeta.Namespace,
			Name:      objMeta.Name,
		})
	}

	return result
}

// resolveProfile returns the profile name for a workload and whether the
// workload should be assigned to a profile. Workload-level label takes
// precedence over namespace-level.
func (w *WorkloadWatcher) resolveProfile(
	gvkr GroupVersionKindResource,
	obj *metav1.PartialObjectMetadata,
	labeledNamespaces map[string]string,
) (string, bool) {
	if profileName := obj.Labels[model.ProfileLabelKey]; profileName != "" {
		if profileName == model.ProfileExcludedValue {
			return "", false
		}
		if _, ok := w.profileStore.Get(profileName); !ok {
			log.Debugf("Profile %s referenced by workload %s/%s/%s not found, skipping", profileName, gvkr.GroupVersionResource.Resource, obj.Namespace, obj.Name)
			return "", false
		}
		return profileName, true
	}

	nsProfile, inLabeledNs := labeledNamespaces[obj.Namespace]
	if !inLabeledNs {
		return "", false
	}
	return nsProfile, true
}
