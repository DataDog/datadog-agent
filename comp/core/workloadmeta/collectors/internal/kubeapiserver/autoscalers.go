// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

// This file builds the mapping from a workload (currently only Deployments) to
// the autoscalers acting on it, and reflects that mapping onto the workload's
// KubernetesDeployment entity and onto the entities of the pods it owns.
//
// Several autoscaler resources feed a single index, so the one-object-to-one-
// entity model of reflectorStore does not apply: N autoscalers collapse into
// one set of kinds on one Deployment entity. Each watched resource therefore
// gets its own cache.Store implementation (autoscalerStore) writing into a
// shared autoscalerIndex, which owns the aggregation and the event emission.

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func shouldCollectAutoscalerTags(cfg config.Reader) bool {
	return cfg.GetBool("cluster_agent.autoscaler_tags.enabled")
}

// autoscalerSpec describes one watched autoscaler resource: how to find it and
// how to read the workload it targets.
type autoscalerSpec struct {
	// gvrString is the "{group}/{resource}" form passed to discovery. The
	// version is resolved to the cluster's preferred one, and a resource the
	// cluster does not have is dropped, so an absent CRD never produces a
	// watch.
	gvrString string
	// kind is the value reported by the kube_autoscaler_kind tag, unless the
	// object turns out to be managed by KEDA.
	kind string
	// targetRefPath is the path within the object's spec holding the reference
	// to the scaled workload. HPAs and WPAs use scaleTargetRef, DPAs and VPAs
	// use targetRef.
	targetRefPath []string
	// kedaAware marks resources KEDA can generate on a user's behalf. Only
	// those are checked for KEDA ownership.
	kedaAware bool
}

func autoscalerSpecs() []autoscalerSpec {
	return []autoscalerSpec{
		{
			gvrString:     kubernetes.HorizontalPodAutoscalerGroupName + "/" + kubernetes.HorizontalPodAutoscalerResourceName,
			kind:          kubernetes.AutoscalerKindHPA,
			targetRefPath: []string{"spec", "scaleTargetRef"},
			kedaAware:     true,
		},
		{
			gvrString:     kubernetes.DatadogGroupName + "/" + kubernetes.WatermarkPodAutoscalerResourceName,
			kind:          kubernetes.AutoscalerKindWPA,
			targetRefPath: []string{"spec", "scaleTargetRef"},
			kedaAware:     true,
		},
		{
			gvrString:     kubernetes.DatadogGroupName + "/" + kubernetes.DatadogPodAutoscalerResourceName,
			kind:          kubernetes.AutoscalerKindDPA,
			targetRefPath: []string{"spec", "targetRef"},
		},
		{
			gvrString:     kubernetes.VerticalPodAutoscalerGroupName + "/" + kubernetes.VerticalPodAutoscalerResourceName,
			kind:          kubernetes.AutoscalerKindVPA,
			targetRefPath: []string{"spec", "targetRef"},
		},
	}
}

// autoscalerRef identifies one autoscaler object within the index. The resource
// is part of the key so that two autoscalers of different kinds sharing a name
// do not collide, and so that a Replace from one resource's reflector only
// reconciles that resource's entries.
type autoscalerRef struct {
	resource  string
	namespace string
	name      string
}

// autoscalerEntry is what the index remembers about one autoscaler object.
type autoscalerEntry struct {
	target kubernetes.WorkloadTarget
	kind   string
}

// autoscalerIndex aggregates every watched autoscaler into a per-workload set
// of autoscaler kinds, and pushes that set into workloadmeta. It is shared by
// all the per-resource stores, so all of its methods take the lock.
type autoscalerIndex struct {
	wlmetaStore workloadmeta.Component

	mu sync.Mutex
	// byAutoscaler is the reverse index. It exists so that an autoscaler whose
	// targetRef is edited to point at a different workload removes itself from
	// the old one.
	byAutoscaler map[autoscalerRef]autoscalerEntry
	byWorkload   map[kubernetes.WorkloadTarget]map[autoscalerRef]string

	// podsByWorkload and workloadByPod track the pods the Cluster Agent knows
	// about, so a change to a workload's autoscalers can be reflected onto its
	// pods. Both are empty unless the pod store is running.
	podsByWorkload map[kubernetes.WorkloadTarget]sets.Set[string]
	workloadByPod  map[string]kubernetes.WorkloadTarget

	// emitted is the set last notified for each workload. It is what makes a
	// resync, or an edit that does not change the target, produce no event.
	emitted map[kubernetes.WorkloadTarget]sets.Set[string]
}

func newAutoscalerIndex(wlmetaStore workloadmeta.Component) *autoscalerIndex {
	return &autoscalerIndex{
		wlmetaStore:    wlmetaStore,
		byAutoscaler:   make(map[autoscalerRef]autoscalerEntry),
		byWorkload:     make(map[kubernetes.WorkloadTarget]map[autoscalerRef]string),
		podsByWorkload: make(map[kubernetes.WorkloadTarget]sets.Set[string]),
		workloadByPod:  make(map[string]kubernetes.WorkloadTarget),
		emitted:        make(map[kubernetes.WorkloadTarget]sets.Set[string]),
	}
}

// kindsForLocked returns the set of autoscaler kinds currently acting on the
// workload, or nil when there are none.
func (i *autoscalerIndex) kindsForLocked(target kubernetes.WorkloadTarget) sets.Set[string] {
	byRef := i.byWorkload[target]
	if len(byRef) == 0 {
		return nil
	}
	kinds := sets.New[string]()
	for _, kind := range byRef {
		kinds.Insert(kind)
	}
	return kinds
}

// setLocked records an autoscaler and returns the workloads whose kind set may
// have changed: the new target, plus the previous one if the autoscaler was
// retargeted.
func (i *autoscalerIndex) setLocked(ref autoscalerRef, entry autoscalerEntry) []kubernetes.WorkloadTarget {
	affected := []kubernetes.WorkloadTarget{entry.target}

	if previous, ok := i.byAutoscaler[ref]; ok && previous.target != entry.target {
		i.removeFromWorkloadLocked(ref, previous.target)
		affected = append(affected, previous.target)
	}

	i.byAutoscaler[ref] = entry
	if i.byWorkload[entry.target] == nil {
		i.byWorkload[entry.target] = make(map[autoscalerRef]string)
	}
	i.byWorkload[entry.target][ref] = entry.kind

	return affected
}

// deleteLocked forgets an autoscaler and returns the workload it was acting on,
// if any.
func (i *autoscalerIndex) deleteLocked(ref autoscalerRef) []kubernetes.WorkloadTarget {
	previous, ok := i.byAutoscaler[ref]
	if !ok {
		return nil
	}
	delete(i.byAutoscaler, ref)
	i.removeFromWorkloadLocked(ref, previous.target)
	return []kubernetes.WorkloadTarget{previous.target}
}

func (i *autoscalerIndex) removeFromWorkloadLocked(ref autoscalerRef, target kubernetes.WorkloadTarget) {
	byRef := i.byWorkload[target]
	if byRef == nil {
		return
	}
	delete(byRef, ref)
	if len(byRef) == 0 {
		delete(i.byWorkload, target)
	}
}

// apply runs a mutation against the index and notifies workloadmeta for every
// workload whose set of autoscaler kinds actually changed.
func (i *autoscalerIndex) apply(mutate func() []kubernetes.WorkloadTarget) {
	i.mu.Lock()

	var events []workloadmeta.CollectorEvent
	seen := make(map[kubernetes.WorkloadTarget]struct{})
	for _, target := range mutate() {
		if _, dup := seen[target]; dup {
			continue
		}
		seen[target] = struct{}{}
		events = append(events, i.eventsForWorkloadLocked(target)...)
	}
	i.mu.Unlock()

	if len(events) > 0 {
		i.wlmetaStore.Notify(events)
	}
}

// eventsForWorkloadLocked builds the workloadmeta events reflecting the current
// state of a workload: one for the workload entity itself and one per pod it
// owns. It returns nothing when the kind set is unchanged since the last
// notification, so resyncs are free.
func (i *autoscalerIndex) eventsForWorkloadLocked(target kubernetes.WorkloadTarget) []workloadmeta.CollectorEvent {
	// Only Deployments have a workloadmeta entity to hang the kinds off today.
	// StatefulSets and Argo Rollouts stay in the index so that adding their
	// entities later is purely additive.
	if target.Kind != kubernetes.DeploymentKind {
		return nil
	}

	kinds := i.kindsForLocked(target)

	previous, everEmitted := i.emitted[target]
	if everEmitted && previous.Equal(kinds) {
		return nil
	}
	if !everEmitted && kinds.Len() == 0 {
		// Nothing was ever published for this workload and there is nothing to
		// publish now: an unset here would be noise.
		return nil
	}

	if kinds.Len() == 0 {
		delete(i.emitted, target)
	} else {
		i.emitted[target] = kinds
	}

	events := kindsEvents(&workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   target.Namespace + "/" + target.Name,
		},
		AutoscalerKinds: kinds,
	})
	for podUID := range i.podsByWorkload[target] {
		events = append(events, podEvents(podUID, kinds)...)
	}

	return events
}

// podEvents builds the events carrying a pod's autoscaler kinds.
func podEvents(podUID string, kinds sets.Set[string]) []workloadmeta.CollectorEvent {
	return kindsEvents(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podUID,
		},
		AutoscalerKinds: kinds,
	})
}

// kindsEvents builds the events publishing an entity's autoscaler kinds under
// this collector's source. entity must carry its ID and AutoscalerKinds only.
//
// Clearing the kinds takes two events, and the order matters. The entity
// usually has another source (the Deployment or Pod reflector), and when one
// source of a multi-source entity is unset, workloadmeta deliberately notifies
// subscribers with the *pre-delete* merged state. A bare Unset would leave the
// tagger holding the old kinds indefinitely, even though the store is correct.
//
//   - The Set with an empty set comes first. It replaces this source's entity,
//     and since no other source ever writes AutoscalerKinds, subscribers see
//     the field cleared.
//   - The Unset then drops this source. Its pre-delete notification is the
//     already-cleared state, so it is harmless, and it removes the entity
//     outright when this source was the only one (an autoscaler targeting a
//     workload that does not exist).
func kindsEvents(entity workloadmeta.Entity) []workloadmeta.CollectorEvent {
	set := workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceKubeAutoscalers,
		Entity: entity,
	}
	if !kindsEmpty(entity) {
		return []workloadmeta.CollectorEvent{set}
	}
	return []workloadmeta.CollectorEvent{
		set,
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceKubeAutoscalers,
			Entity: entity,
		},
	}
}

func kindsEmpty(entity workloadmeta.Entity) bool {
	switch e := entity.(type) {
	case *workloadmeta.KubernetesDeployment:
		return e.AutoscalerKinds.Len() == 0
	case *workloadmeta.KubernetesPod:
		return e.AutoscalerKinds.Len() == 0
	}
	return false
}

// autoscalerStore implements cache.Store for a single autoscaler resource,
// translating watch events into index mutations.
type autoscalerStore struct {
	index *autoscalerIndex
	spec  autoscalerSpec
	gvr   schema.GroupVersionResource

	mu        sync.Mutex
	hasSynced bool
}

// parse extracts the index key and value from an autoscaler object, reporting
// false when the object does not target a workload we track.
func (s *autoscalerStore) parse(obj interface{}) (autoscalerRef, autoscalerEntry, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.Debugf("Unexpected object type %T watching %s", obj, s.gvr.Resource)
		return autoscalerRef{}, autoscalerEntry{}, false
	}

	ref := autoscalerRef{
		resource:  s.gvr.Resource,
		namespace: u.GetNamespace(),
		name:      u.GetName(),
	}

	target, ok := targetFromObject(u, s.spec.targetRefPath)
	if !ok {
		return ref, autoscalerEntry{}, false
	}

	kind := s.spec.kind
	if s.spec.kedaAware && isKedaManaged(u) {
		kind = kubernetes.AutoscalerKindKeda
	}

	return ref, autoscalerEntry{target: target, kind: kind}, true
}

// targetFromObject reads the workload reference an autoscaler points at. The
// reference is always in the autoscaler's own namespace.
func targetFromObject(u *unstructured.Unstructured, path []string) (kubernetes.WorkloadTarget, bool) {
	kind, _, err := unstructured.NestedString(u.Object, append(append([]string{}, path...), "kind")...)
	if err != nil || kind == "" {
		return kubernetes.WorkloadTarget{}, false
	}
	name, _, err := unstructured.NestedString(u.Object, append(append([]string{}, path...), "name")...)
	if err != nil || name == "" {
		return kubernetes.WorkloadTarget{}, false
	}

	return kubernetes.WorkloadTarget{
		Kind:      kind,
		Namespace: u.GetNamespace(),
		Name:      name,
	}, true
}

// isKedaManaged reports whether an autoscaler was generated by KEDA on behalf
// of a ScaledObject. KEDA sets an owner reference on the autoscalers it
// creates; the managed-by label is checked as well because it is what the
// external metrics provider already keys on.
func isKedaManaged(u *unstructured.Unstructured) bool {
	for _, owner := range u.GetOwnerReferences() {
		if owner.Kind == kubernetes.KedaScaledObjectKind {
			return true
		}
	}
	return u.GetLabels()[kubernetes.ManagedByLabelKey] == kubernetes.KedaManagedByLabelValue
}

// Add implements cache.Store.
func (s *autoscalerStore) Add(obj interface{}) error {
	s.markSynced()

	ref, entry, ok := s.parse(obj)
	if !ok {
		// The object may previously have had a valid target and lost it, so
		// treat an unparseable target as a removal rather than ignoring it.
		if ref != (autoscalerRef{}) {
			s.index.apply(func() []kubernetes.WorkloadTarget { return s.index.deleteLocked(ref) })
		}
		return nil
	}

	s.index.apply(func() []kubernetes.WorkloadTarget { return s.index.setLocked(ref, entry) })
	return nil
}

// Update implements cache.Store.
func (s *autoscalerStore) Update(obj interface{}) error {
	return s.Add(obj)
}

// Delete implements cache.Store.
func (s *autoscalerStore) Delete(obj interface{}) error {
	s.markSynced()

	ref, _, _ := s.parse(obj)
	if ref == (autoscalerRef{}) {
		return nil
	}

	s.index.apply(func() []kubernetes.WorkloadTarget { return s.index.deleteLocked(ref) })
	return nil
}

// Replace implements cache.Store. It reconciles only the entries belonging to
// this store's resource, leaving the other autoscaler kinds in the shared index
// untouched.
func (s *autoscalerStore) Replace(list []interface{}, _ string) error {
	parsed := make(map[autoscalerRef]autoscalerEntry, len(list))
	for _, obj := range list {
		ref, entry, ok := s.parse(obj)
		if !ok {
			continue
		}
		parsed[ref] = entry
	}

	s.index.apply(func() []kubernetes.WorkloadTarget {
		var affected []kubernetes.WorkloadTarget

		for ref := range s.index.byAutoscaler {
			if ref.resource != s.gvr.Resource {
				continue
			}
			if _, stillThere := parsed[ref]; stillThere {
				continue
			}
			affected = append(affected, s.index.deleteLocked(ref)...)
		}

		for ref, entry := range parsed {
			affected = append(affected, s.index.setLocked(ref, entry)...)
		}

		return affected
	})

	s.markSynced()
	return nil
}

func (s *autoscalerStore) markSynced() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasSynced = true
}

// HasSynced implements cache.Store.
func (s *autoscalerStore) HasSynced() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasSynced
}

// List is not implemented
func (s *autoscalerStore) List() []interface{} { panic("not implemented") }

// ListKeys is not implemented
func (s *autoscalerStore) ListKeys() []string { panic("not implemented") }

// Get is not implemented
func (s *autoscalerStore) Get(_ interface{}) (interface{}, bool, error) { panic("not implemented") }

// GetByKey is not implemented
func (s *autoscalerStore) GetByKey(_ string) (interface{}, bool, error) { panic("not implemented") }

// Resync is not implemented
func (s *autoscalerStore) Resync() error { panic("not implemented") }

func newAutoscalerStore(index *autoscalerIndex, client dynamic.Interface, spec autoscalerSpec, gvr schema.GroupVersionResource) (*cache.Reflector, *autoscalerStore) {
	listerWatcher := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	store := &autoscalerStore{index: index, spec: spec, gvr: gvr}
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store
}

// watchPods keeps the index's view of pods in sync with workloadmeta, so that
// autoscaler kinds can be reflected onto the pods of an autoscaled workload as
// well as onto the workload itself.
//
// It only ever sees pods when the Cluster Agent runs a pod store. When it does
// not, the mapping is still published on the Deployment entity, and node
// agents do their own pod join locally.
func (i *autoscalerIndex) watchPods(ctx context.Context, wlmetaStore workloadmeta.Component) {
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceKubeAPIServer).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	ch := wlmetaStore.Subscribe(componentName+"-autoscalers", workloadmeta.NormalPriority, filter)
	defer wlmetaStore.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case bundle, ok := <-ch:
			if !ok {
				return
			}
			i.handlePodEvents(bundle)
		}
	}
}

func (i *autoscalerIndex) handlePodEvents(bundle workloadmeta.EventBundle) {
	// Nothing downstream depends on this processing, so ack immediately.
	bundle.Acknowledge()

	var events []workloadmeta.CollectorEvent

	i.mu.Lock()
	for _, event := range bundle.Events {
		pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			target, found := workloadTargetForPod(pod)
			if !found {
				i.forgetPodLocked(pod.ID)
				continue
			}
			if previous, known := i.workloadByPod[pod.ID]; known && previous != target {
				i.forgetPodLocked(pod.ID)
			}
			i.workloadByPod[pod.ID] = target
			if i.podsByWorkload[target] == nil {
				i.podsByWorkload[target] = sets.New[string]()
			}
			i.podsByWorkload[target].Insert(pod.ID)

			// Publish straight away rather than waiting for the next autoscaler
			// change: this is the path that tags a pod the moment it appears.
			if kinds := i.kindsForLocked(target); kinds.Len() > 0 {
				events = append(events, podEvents(pod.ID, kinds)...)
			}

		case workloadmeta.EventTypeUnset:
			i.forgetPodLocked(pod.ID)
		}
	}
	i.mu.Unlock()

	if len(events) > 0 {
		i.wlmetaStore.Notify(events)
	}
}

func (i *autoscalerIndex) forgetPodLocked(podUID string) {
	target, known := i.workloadByPod[podUID]
	if !known {
		return
	}
	delete(i.workloadByPod, podUID)

	pods := i.podsByWorkload[target]
	if pods == nil {
		return
	}
	pods.Delete(podUID)
	if pods.Len() == 0 {
		delete(i.podsByWorkload, target)
	}
}

// workloadTargetForPod resolves the workload controller owning a pod. A pod
// with several owners is not something Kubernetes produces for the controllers
// we track, so the first resolvable owner wins.
func workloadTargetForPod(pod *workloadmeta.KubernetesPod) (kubernetes.WorkloadTarget, bool) {
	for _, owner := range pod.Owners {
		if target, ok := kubernetes.ResolveWorkloadTarget(pod.Namespace, owner.Kind, owner.Name, pod.Labels); ok {
			return target, true
		}
	}
	return kubernetes.WorkloadTarget{}, false
}

// startAutoscalerStores wires up one reflector per autoscaler resource the
// cluster actually has, all feeding a single index.
//
// Resources whose CRD is absent are dropped by discovery, so they never produce
// a reflector and never end up in the caller's readiness set. That matters:
// an informer that can never sync would otherwise block startup indefinitely.
func startAutoscalerStores(ctx context.Context, index *autoscalerIndex, discovery discovery.DiscoveryInterface, client dynamic.Interface) []cache.ResourceEventHandlerRegistration {
	var stores []cache.ResourceEventHandlerRegistration

	for _, spec := range autoscalerSpecs() {
		gvrs, err := getGVRsForRequestedResources(discovery, []string{spec.gvrString})
		if err != nil {
			log.Errorf("Failed to discover autoscaler resource %s: %v", spec.gvrString, err)
			continue
		}
		if len(gvrs) == 0 {
			log.Infof("Autoscaler resource %s is not available in this cluster, skipping it for autoscaler tags", spec.gvrString)
			continue
		}

		for _, gvr := range gvrs {
			reflector, store := newAutoscalerStore(index, client, spec, gvr)
			stores = append(stores, store)
			go reflector.Run(ctx.Done())
			log.Infof("Watching %s for autoscaler tags", gvr.String())
		}
	}

	return stores
}
