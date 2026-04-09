// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"sync"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/client-go/tools/cache"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	syncerStoreID autoscaling.SenderID = "prof-s"

	syncerReconcilePeriod = 1 * time.Minute
)

// AutoscalerSyncer maintains consistency between the profile store (workload
// references set by the WorkloadWatcher) and the DPA store. It registers as an
// observer on both stores and reconciles whenever either changes.
//
// Internal state:
//   - dpaOwnership maps each DPA store key to the profile name that owns it.
//     This is the single source of truth for what the syncer has created.
type AutoscalerSyncer struct {
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal]
	dpaStore     *autoscaling.Store[model.PodAutoscalerInternal]
	isLeader     func() bool

	mu           sync.Mutex
	dpaOwnership map[string]string // dpa store key → profile name

	reconcileCh chan struct{}

	// readyDeps are checked before the syncer starts reconciling. This
	// prevents acting on incomplete data during startup and is required for
	// correct orphan cleanup (see rebuildOwnership).
	readyDeps []cache.InformerSynced
}

// NewAutoscalerSyncer creates a new AutoscalerSyncer and registers observers.
// readyDeps are polled before the syncer starts reconciling; it blocks in Run
// until every dependency returns true. Typical deps are
// WorkloadWatcher.HasSynced and the profile controller's HasSynced.
func NewAutoscalerSyncer(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
	dpaStore *autoscaling.Store[model.PodAutoscalerInternal],
	isLeader func() bool,
	readyDeps ...cache.InformerSynced,
) *AutoscalerSyncer {
	s := &AutoscalerSyncer{
		profileStore: profileStore,
		dpaStore:     dpaStore,
		isLeader:     isLeader,
		dpaOwnership: make(map[string]string),
		reconcileCh:  make(chan struct{}, 1),
		readyDeps:    readyDeps,
	}

	// Only observe the profile store. DPA store changes (values updates, scaling
	// events) are irrelevant and would trigger expensive reconciles every ~30s.
	// Conflict with user-created DPAs is detected by the periodic safety-net reconcile.
	profileStore.RegisterObserver(autoscaling.Observer{
		SetFunc:    func(_ string, sender autoscaling.SenderID) { s.enqueue(sender) },
		DeleteFunc: func(_ string, sender autoscaling.SenderID) { s.enqueue(sender) },
	})

	return s
}

func (s *AutoscalerSyncer) enqueue(sender autoscaling.SenderID) {
	if sender == syncerStoreID {
		return
	}
	select {
	case s.reconcileCh <- struct{}{}:
	default:
	}
}

// Run starts the syncer loop. It blocks until ctx is cancelled.
// Before entering the main loop, Run waits for readiness dependencies, then
// rebuilds dpaOwnership from the DPA store so that DPAs created in a previous
// lifecycle are correctly tracked (and cleaned up if no longer desired).
func (s *AutoscalerSyncer) Run(ctx context.Context) {
	if len(s.readyDeps) > 0 {
		log.Info("Waiting for dependencies to be ready")
		if !cache.WaitForCacheSync(ctx.Done(), s.readyDeps...) {
			log.Error("Context cancelled while waiting for dependencies")
			return
		}
		log.Info("All dependencies ready")
	}

	s.rebuildOwnership()

	ticker := time.NewTicker(syncerReconcilePeriod)
	defer ticker.Stop()

	log.Info("Starting autoscaler syncer")

	// Perform an initial reconcile immediately so the desired-vs-ownership diff
	// runs as soon as data is ready (don't wait for the 1-minute ticker).
	if s.isLeader() {
		s.reconcile()
	}

	for {
		select {
		case <-s.reconcileCh:
			if s.isLeader() {
				s.reconcile()
			}
		case <-ticker.C:
			if s.isLeader() {
				s.reconcile()
			}
		case <-ctx.Done():
			return
		}
	}
}

// rebuildOwnership scans the DPA store for profile-managed DPAs and populates
// dpaOwnership so the syncer knows what it "owns" from a previous lifecycle.
// Without this, DPAs orphaned while the agent was down (e.g. a workload label
// was removed) would never be cleaned up.
//
// Must be called after readiness deps are met and before the first reconcile.
func (s *AutoscalerSyncer) rebuildOwnership() {
	s.mu.Lock()
	defer s.mu.Unlock()

	profileManagedDPAs := s.dpaStore.GetFiltered(func(pai model.PodAutoscalerInternal) bool {
		return pai.IsProfileManaged() && !pai.Deleted()
	})

	for _, pai := range profileManagedDPAs {
		key := pai.Namespace() + "/" + pai.Name()
		s.dpaOwnership[key] = pai.ProfileName()
	}

	log.Infof("Rebuilt ownership from DPA store, %d profile-managed DPAs", len(s.dpaOwnership))
}

// desiredDPA holds the information needed to create or update a single DPA entry.
type desiredDPA struct {
	profileName  string
	ref          model.NamespacedObjectReference
	template     *datadoghq.DatadogPodAutoscalerTemplate
	templateHash string
	burstable    bool
}

// reconcile performs a full sync between the profile store and the DPA store.
func (s *AutoscalerSyncer) reconcile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	desired := s.buildDesiredState()
	s.resolveConflicts(desired)
	s.applyChanges(desired)
}

// buildDesiredState builds the map of DPA store keys to desired DPA entries
// from all valid profiles with workload references.
func (s *AutoscalerSyncer) buildDesiredState() map[string]desiredDPA {
	desired := make(map[string]desiredDPA)

	profiles := s.profileStore.GetAll()
	for _, profileInternal := range profiles {
		if !profileInternal.Valid() || profileInternal.Template() == nil {
			continue
		}
		burstable := profileInternal.Burstable()
		templateHash := profileInternal.TemplateHash()
		if burstable {
			templateHash += "-burstable"
		}
		for dpaKey, ref := range profileInternal.Workloads() {
			desired[dpaKey] = desiredDPA{
				profileName:  profileInternal.Name(),
				ref:          ref,
				template:     profileInternal.Template(),
				templateHash: templateHash,
				burstable:    burstable,
			}
		}
	}

	return desired
}

// resolveConflicts removes entries from desired that collide with non-profile-managed DPAs.
// Two cases are handled:
//
//  1. Orphan: a DPA we previously managed is no longer profile-managed (customer
//     removed the profile label). The conflicting key matches the user DPA key.
//     We release ownership without deletion so the DPA continues standalone.
//
//  2. Conflict: a separate user-created DPA targets the same workload. The keys
//     differ. We mark the profile-managed DPA deleted and release ownership.
func (s *AutoscalerSyncer) resolveConflicts(desired map[string]desiredDPA) {
	workloadToDesired := make(map[string]string, len(desired))
	for dpaKey, d := range desired {
		wKey := d.ref.Namespace + "/" + d.ref.Kind + "/" + d.ref.Name
		workloadToDesired[wKey] = dpaKey
	}

	if len(workloadToDesired) == 0 {
		return
	}

	nonProfileDPAs := s.dpaStore.GetFiltered(func(pai model.PodAutoscalerInternal) bool {
		return !pai.IsProfileManaged() && pai.Spec() != nil && !pai.Deleted()
	})

	for _, dpa := range nonProfileDPAs {
		wKey := dpa.Namespace() + "/" + dpa.Spec().TargetRef.Kind + "/" + dpa.Spec().TargetRef.Name
		desiredKey, ok := workloadToDesired[wKey]
		if !ok {
			continue
		}

		delete(desired, desiredKey)

		if _, owned := s.dpaOwnership[desiredKey]; !owned {
			continue
		}

		dpaKey := dpa.Namespace() + "/" + dpa.Name()
		if desiredKey == dpaKey {
			log.Infof("DPA %s orphaned (profile label removed), releasing ownership", desiredKey)
		} else {
			log.Infof("User DPA %s conflicts with profile DPA %s, deleting generated DPA", dpaKey, desiredKey)
			s.deleteDPA(desiredKey)
		}
		delete(s.dpaOwnership, desiredKey)
	}
}

// applyChanges diffs the desired state against dpaOwnership and applies
// creates, updates, and deletes to the DPA store.
func (s *AutoscalerSyncer) applyChanges(desired map[string]desiredDPA) {
	for dpaKey, profileName := range s.dpaOwnership {
		if _, ok := desired[dpaKey]; !ok {
			log.Infof("DPA %s (profile %s) no longer desired, marking deleted", dpaKey, profileName)
			s.deleteDPA(dpaKey)
			delete(s.dpaOwnership, dpaKey)
		}
	}

	for dpaKey, d := range desired {
		s.ensureDPA(dpaKey, d)
		s.dpaOwnership[dpaKey] = d.profileName
	}
}

// ensureDPA creates a DPA if it does not exist, or updates it if the profile
// or template has changed. No-ops when nothing changed.
func (s *AutoscalerSyncer) ensureDPA(dpaKey string, d desiredDPA) {
	targetRef := buildTargetRef(d.ref)

	pai, found, unlock := s.dpaStore.LockRead(dpaKey, true)
	if !found {
		_, name, _ := cache.SplitMetaNamespaceKey(dpaKey)
		log.Infof("Creating DPA %s for profile %s", dpaKey, d.profileName)
		pai = model.NewPodAutoscalerFromProfile(d.ref.Namespace, name, d.profileName, d.template, targetRef, d.templateHash, d.burstable)
		s.dpaStore.UnlockSet(dpaKey, pai, syncerStoreID)
		return
	}

	if !pai.IsProfileManaged() {
		unlock()
		return
	}

	if pai.ProfileName() == d.profileName && pai.DesiredProfileTemplateHash() == d.templateHash {
		unlock()
		return
	}

	pai.UpdateFromProfile(d.profileName, d.template, targetRef, d.templateHash, d.burstable)
	s.dpaStore.UnlockSet(dpaKey, pai, syncerStoreID)
}

// deleteDPA marks a DPA entry as deleted in the store.
func (s *AutoscalerSyncer) deleteDPA(dpaKey string) {
	pai, found, unlock := s.dpaStore.LockRead(dpaKey, false)
	if !found {
		unlock()
		return
	}

	pai.SetDeleted()
	s.dpaStore.UnlockSet(dpaKey, pai, syncerStoreID)
}

func buildTargetRef(ref model.NamespacedObjectReference) autoscalingv2.CrossVersionObjectReference {
	return autoscalingv2.CrossVersionObjectReference{
		Kind:       ref.Kind,
		Name:       ref.Name,
		APIVersion: ref.APIVersion(),
	}
}
