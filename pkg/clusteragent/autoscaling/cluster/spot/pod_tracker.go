// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type pendingSpotPod struct {
	topLevelOwner objectRef
	name          string
	createdAt     time.Time
}

// ownerPodSet tracks spot and on-demand pods and in-flight admission counts for a single direct owner.
type ownerPodSet struct {
	config     workloadSpotConfig
	lastUpdate time.Time

	spotUIDs               map[string]podInfo
	onDemandUIDs           map[string]podInfo
	admissionSpotCount     int
	admissionOnDemandCount int
}

// podInfo holds pod metadata.
type podInfo struct {
	name  string
	phase string
}

// podTracker keeps track of running pods and in-flight admissions per workload.
// It is populated from two sources:
//   - The admission webhook calls admitNewPod on every pod CREATE request and records
//     the spot/on-demand decision before the pod is visible in Kubernetes.
//   - The Kubernetes watch calls addedOrUpdated/deleted as pods appear
//     converting in-flight admission counts into UID-keyed records.
//
// Pods are grouped first by top-level owner (e.g. Deployment) and then by direct owner
// (e.g. ReplicaSet). This enables O(1) per-workload operations.
type podTracker struct {
	defaultConfig workloadSpotConfig
	configSource  func(objectRef) (workloadSpotConfig, bool)
	telemetry     *telemetry

	mu              sync.RWMutex
	podSets         map[objectRef]map[objectRef]*ownerPodSet
	pendingSpotPods map[string]pendingSpotPod
}

func newPodTracker(defaultConfig workloadSpotConfig, configSource func(objectRef) (workloadSpotConfig, bool), tel *telemetry) *podTracker {
	return &podTracker{
		defaultConfig:   defaultConfig,
		configSource:    configSource,
		telemetry:       tel,
		podSets:         make(map[objectRef]map[objectRef]*ownerPodSet),
		pendingSpotPods: make(map[string]pendingSpotPod),
	}
}

// admitNewPod decides whether the new pod should be spot-assigned using
// the per-owner config and returns true if the pod was assigned to spot.
func (t *podTracker) admitNewPod(o podOwnership) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	ps, ok := t.getPodSetLocked(o)
	if !ok {
		return false
	}

	total := ps.totalCount()
	spot := ps.spotCount()
	onDemand := total - spot

	if onDemand < ps.config.minOnDemand {
		log.Debugf("Skipping pod for %s: on-demand minimum not met (%d < %d), total: %d, spot: %d", o.directOwner, onDemand, ps.config.minOnDemand, total, spot)
		return ps.admit(false)
	}

	desiredSpot := (total + 1) * ps.config.percentage / 100
	if spot >= desiredSpot {
		log.Debugf("Skipping pod for %s: desired spot reached (%d >= %d), total: %d", o.directOwner, spot, desiredSpot, total)
		return ps.admit(false)
	}

	log.Debugf("Assigning pod for %s to spot (%d of desired %d spot, %d on-demand), total: %d", o.directOwner, spot, desiredSpot, onDemand, total)
	return ps.admit(true)
}

// admitNewOnDemandPod records an on-demand admission for the pod ownership.
func (t *podTracker) admitNewOnDemandPod(o podOwnership) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ps, ok := t.getPodSetLocked(o); ok {
		ps.admit(false)
	}
}

// addedOrUpdated updates tracking state when a pod is added or updated.
func (t *podTracker) addedOrUpdated(pod *workloadmeta.KubernetesPod) {
	o, ok := resolveWLMPodOwnership(pod)
	if !ok {
		log.Debugf("Ignoring pod %s: cannot resolve ownership", pod.ID)
		return
	}

	isSpot := isSpotAssigned(pod)

	log.Debugf("Pod %s added/updated for owner %s (phase=%s, spot=%v)", pod.ID, o.directOwner, pod.Phase, isSpot)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Terminal pods are treated as removed to free their spot/on-demand slot
	// before replacement pods are admitted.
	if pod.Phase == string(corev1.PodSucceeded) || pod.Phase == string(corev1.PodFailed) {
		t.deletePodLocked(o, pod.ID)
		return
	}

	ps, ok := t.getPodSetLocked(o)
	if !ok {
		return
	}
	ps.track(pod.ID, isSpot, podInfo{name: pod.Name, phase: pod.Phase}, time.Now())
	t.updateWorkloadMetricsLocked(o.topLevelOwner)

	if isSpot {
		if pod.Phase == string(corev1.PodPending) {
			if _, exists := t.pendingSpotPods[pod.ID]; !exists {
				createdAt := time.Now()
				if !pod.CreationTimestamp.IsZero() {
					createdAt = pod.CreationTimestamp
				}
				t.pendingSpotPods[pod.ID] = pendingSpotPod{topLevelOwner: o.topLevelOwner, name: pod.Name, createdAt: createdAt}
				log.Debugf("Tracking pending spot pod %s", pod.ID)
			}
		} else {
			if prev, exists := t.pendingSpotPods[pod.ID]; exists {
				delete(t.pendingSpotPods, pod.ID)
				// Run in a goroutine to not hold mutex longer than necessary
				go t.telemetry.observePendingSeconds(time.Since(prev.createdAt))
			}
		}
	}
}

// deleted updates tracking state when a pod is deleted.
func (t *podTracker) deleted(pod *workloadmeta.KubernetesPod) {
	o, ok := resolveWLMPodOwnership(pod)
	if !ok {
		log.Debugf("Ignoring pod %s: cannot resolve ownership", pod.ID)
		return
	}

	log.Debugf("Pod %s deleted for owner %s (spot=%v)", pod.ID, o.directOwner, isSpotAssigned(pod))

	t.deletePod(o, pod.ID)
}

// deletePod deletes pod by ownership and uid.
func (t *podTracker) deletePod(o podOwnership, uid string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.deletePodLocked(o, uid)
}

// deletePodLocked deletes a pod from podSets. Must be called with t.mu held.
func (t *podTracker) deletePodLocked(o podOwnership, uid string) {
	if owners, ok := t.podSets[o.topLevelOwner]; ok {
		if ps, ok := owners[o.directOwner]; ok {
			if ps.delete(uid, time.Now()) {
				delete(owners, o.directOwner)
			}
		}
		if len(owners) == 0 {
			delete(t.podSets, o.topLevelOwner)
		}
	}
	delete(t.pendingSpotPods, uid)
	t.updateWorkloadMetricsLocked(o.topLevelOwner)
}

// getPodSetLocked returns the ownerPodSet for the given ownership, creating it if absent,
// and always applies the latest config from configSource to the returned pod set.
// When configSource has no entry for the top-level owner it removes
// tracking state for the owner and returns (nil, false).
// Must be called with t.mu held.
func (t *podTracker) getPodSetLocked(o podOwnership) (*ownerPodSet, bool) {
	cfg, hasConfig := t.configSource(o.topLevelOwner)
	if !hasConfig {
		t.untrackLocked(o.topLevelOwner)
		return nil, false
	}

	owners, ok := t.podSets[o.topLevelOwner]
	if !ok {
		owners = make(map[objectRef]*ownerPodSet)
		t.podSets[o.topLevelOwner] = owners
	}

	ps, exists := owners[o.directOwner]
	if !exists {
		ps = t.newOwnerPodSet()
		owners[o.directOwner] = ps
	}
	ps.config = cfg
	return ps, true
}

// getPodToDelete returns the top-level owner, uid, name, and whether the selected pod is
// spot-assigned, for a pod to delete to make progress toward the desired config across all
// tracked owners. Returns zero values if no deletion is needed.
// When a pod is selected, lastUpdate is stamped on its owner's ownerPodSet to prevent selecting the
// same owner again before the deletion takes effect.
func (t *podTracker) getPodToDelete(rebalanceStabilizationPeriod time.Duration) (objectRef, string, string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	lastUpdatedBefore := now.Add(-rebalanceStabilizationPeriod)
	for topLevel, owners := range t.podSets {
		cfg, ok := t.configSource(topLevel)
		if !ok {
			t.untrackLocked(topLevel)
			continue
		}
		for _, ps := range owners {
			ps.config = cfg
			if ps.config.isDisabled(now) {
				continue
			}
			if uid, name, isSpot := ps.getPodToDelete(lastUpdatedBefore); uid != "" {
				ps.lastUpdate = now // suppress re-selection until stabilization period elapses
				return topLevel, uid, name, isSpot
			}
		}
	}
	return objectRef{}, "", "", false
}

// getPendingSpotPods returns spot-assigned pods that have been pending since before the given time keyed by pod UID.
func (t *podTracker) getPendingSpotPods(since time.Time) map[string]pendingSpotPod {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]pendingSpotPod)
	for uid, info := range t.pendingSpotPods {
		if !since.Before(info.createdAt) {
			result[uid] = info
		}
	}
	return result
}

// deletePendingSpotPod removes a single pod from pending tracking.
func (t *podTracker) deletePendingSpotPod(uid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pendingSpotPods, uid)
}

// untrack removes all tracking state for the given top-level owner.
func (t *podTracker) untrack(topLevelOwner objectRef) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.untrackLocked(topLevelOwner)
}

// untrackLocked removes all tracking state for the given top-level owner.
// Must be called with t.mu held.
func (t *podTracker) untrackLocked(topLevelOwner objectRef) {
	delete(t.podSets, topLevelOwner)
	for uid, p := range t.pendingSpotPods {
		if p.topLevelOwner == topLevelOwner {
			delete(t.pendingSpotPods, uid)
		}
	}
	t.telemetry.deleteWorkload(topLevelOwner)
}

func (t *podTracker) newOwnerPodSet() *ownerPodSet {
	return &ownerPodSet{
		config:       t.defaultConfig,
		spotUIDs:     make(map[string]podInfo),
		onDemandUIDs: make(map[string]podInfo),
	}
}

// updateWorkloadMetricsLocked updates the stored workload snapshot for the given top-level owner.
// Must be called with t.mu held.
func (t *podTracker) updateWorkloadMetricsLocked(topLevelOwner objectRef) {
	_, hasConfig := t.configSource(topLevelOwner)
	if !hasConfig {
		t.telemetry.deleteWorkload(topLevelOwner)
		return
	}
	owners, ok := t.podSets[topLevelOwner]
	if !ok {
		t.telemetry.deleteWorkload(topLevelOwner)
		return
	}
	snap := workloadSnapshot{ref: topLevelOwner}
	for _, ps := range owners {
		snap.spot += len(ps.spotUIDs)
		snap.onDemand += len(ps.onDemandUIDs)
		excessSpot, excessOnDemand := ps.excess()
		snap.excessSpot += excessSpot
		snap.excessOnDemand += excessOnDemand
	}
	t.telemetry.observeWorkload(snap)
}

// admit increments the in-flight admission count for the given spot/on-demand decision and returns isSpot.
func (ps *ownerPodSet) admit(isSpot bool) bool {
	if isSpot {
		ps.admissionSpotCount++
	} else {
		ps.admissionOnDemandCount++
	}
	return isSpot
}

// track upserts the pod UID with its info, decrementing the in-flight admission count on first appearance.
func (ps *ownerPodSet) track(uid string, isSpot bool, info podInfo, now time.Time) {
	if isSpot {
		if _, exists := ps.spotUIDs[uid]; !exists {
			if ps.admissionSpotCount > 0 {
				ps.admissionSpotCount--
			}
		}
		ps.spotUIDs[uid] = info
	} else {
		if _, exists := ps.onDemandUIDs[uid]; !exists {
			if ps.admissionOnDemandCount > 0 {
				ps.admissionOnDemandCount--
			}
		}
		ps.onDemandUIDs[uid] = info
	}
	ps.lastUpdate = now
}

// getPodToDelete returns the uid, name, and whether the selected pod is spot-assigned,
// to make progress toward the desired config. Returns empty strings if no deletion is needed.
func (ps *ownerPodSet) getPodToDelete(lastUpdatedBefore time.Time) (string, string, bool) {
	if ps.hasAdmissions() {
		return "", "", false
	}

	if ps.lastUpdate.After(lastUpdatedBefore) {
		return "", "", false
	}

	if ps.hasPending() {
		return "", "", false
	}

	excessSpot, excessOnDemand := ps.excess()
	if excessSpot > 0 {
		uid, name := pickPod(ps.spotUIDs)
		return uid, name, true
	}
	if excessOnDemand > 0 {
		uid, name := pickPod(ps.onDemandUIDs)
		return uid, name, false
	}
	return "", "", false
}

// excess returns the number of pods on each capacity type that the rebalancer would have to evict to converge.
// By construction at most one of the two is non-zero.
func (ps *ownerPodSet) excess() (excessSpot, excessOnDemand int) {
	spot, onDemand := len(ps.spotUIDs), len(ps.onDemandUIDs)

	if onDemand < ps.config.minOnDemand {
		return ps.config.minOnDemand - onDemand, 0
	}

	total := spot + onDemand
	desiredSpot := total * ps.config.percentage / 100
	if spot > desiredSpot {
		return spot - desiredSpot, 0
	}

	desiredOnDemand := max(total-desiredSpot, ps.config.minOnDemand)
	if onDemand > desiredOnDemand {
		return 0, onDemand - desiredOnDemand
	}

	return 0, 0
}

// hasAdmissions returns true if pod set has admitted but not yet tracked pods.
func (ps *ownerPodSet) hasAdmissions() bool {
	return ps.admissionSpotCount > 0 || ps.admissionOnDemandCount > 0
}

// hasPending returns true if any tracked pod is in PodPending phase.
func (ps *ownerPodSet) hasPending() bool {
	const pending = string(corev1.PodPending)
	for _, info := range ps.spotUIDs {
		if info.phase == pending {
			return true
		}
	}
	for _, info := range ps.onDemandUIDs {
		if info.phase == pending {
			return true
		}
	}
	return false
}

// pickPod returns the uid and name of a random pod from uids.
func pickPod(uids map[string]podInfo) (string, string) {
	for uid, info := range uids {
		return uid, info.name
	}
	return "", ""
}

// delete removes pod by uid, updates lastUpdate, and returns true if it tracks no more pods including in-flight admissions.
func (ps *ownerPodSet) delete(uid string, now time.Time) bool {
	delete(ps.spotUIDs, uid)
	delete(ps.onDemandUIDs, uid)
	ps.lastUpdate = now
	return len(ps.spotUIDs) == 0 && len(ps.onDemandUIDs) == 0 && ps.admissionSpotCount == 0 && ps.admissionOnDemandCount == 0
}

// totalCount returns the total number of pods including in-flight admissions.
func (ps *ownerPodSet) totalCount() int {
	return len(ps.spotUIDs) + len(ps.onDemandUIDs) + ps.admissionSpotCount + ps.admissionOnDemandCount
}

// spotCount returns the number of spot-assigned pods including in-flight spot admissions.
func (ps *ownerPodSet) spotCount() int {
	return len(ps.spotUIDs) + ps.admissionSpotCount
}

func isSpotAssigned(pod *workloadmeta.KubernetesPod) bool {
	return pod.Labels[SpotAssignedLabel] == SpotAssignedLabelValue
}
