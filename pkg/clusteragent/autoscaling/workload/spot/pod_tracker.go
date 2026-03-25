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
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type pendingSpotPod struct {
	owner     ownerKey
	namespace string
	name      string
	createdAt time.Time
}

// spotConfig holds per-owner spot scheduling parameters.
type spotConfig struct {
	percentage  int
	minOnDemand int
}

// pods tracks spot and on-demand pods and in-flight admission counts for the same owner.
type pods struct {
	config     spotConfig
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

// podTracker keeps track of pods per owner.
type podTracker struct {
	clock         clock.Clock
	defaultConfig spotConfig

	mu sync.RWMutex
	// podsPerOwner groups pods and in-flight admission counts by owner.
	podsPerOwner map[ownerKey]*pods
	// pendingSpotPods tracks spot-assigned pods that are pending scheduling, keyed by pod UID.
	pendingSpotPods map[string]pendingSpotPod
}

func newPodTracker(clk clock.Clock, defaultConfig spotConfig) *podTracker {
	return &podTracker{
		clock:           clk,
		defaultConfig:   defaultConfig,
		podsPerOwner:    make(map[ownerKey]*pods),
		pendingSpotPods: make(map[string]pendingSpotPod),
	}
}

// admitNewPod decides whether the new pod should be spot-assigned using
// the per-owner config and returns true if the pod was assigned to spot.
func (t *podTracker) admitNewPod(owner ownerKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	pods := t.getPodsLocked(owner)

	total := pods.totalCount()
	spot := pods.spotCount()
	onDemand := total - spot

	if onDemand < pods.config.minOnDemand {
		log.Debugf("Skipping pod for %s: on-demand minimum not met (%d < %d), total: %d, spot: %d", owner, onDemand, pods.config.minOnDemand, total, spot)
		return pods.admit(false)
	}

	desiredSpot := (total + 1) * pods.config.percentage / 100
	if spot >= desiredSpot {
		log.Debugf("Skipping pod for %s: desired spot reached (%d >= %d), total: %d", owner, spot, desiredSpot, total)
		return pods.admit(false)
	}

	log.Debugf("Assigning pod for %s to spot (%d of desired %d spot, %d on-demand), total: %d", owner, spot, desiredSpot, onDemand, total)
	return pods.admit(true)
}

// updateConfig updates the spot scheduling config for owner.
func (t *podTracker) updateConfig(owner ownerKey, config spotConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.getPodsLocked(owner).config = config
}

// admitNewOnDemandPod records an on-demand admission for owner.
func (t *podTracker) admitNewOnDemandPod(owner ownerKey) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.getPodsLocked(owner).admit(false)
}

// addedOrUpdated updates tracking state when a pod is added or updated.
func (t *podTracker) addedOrUpdated(pod *workloadmeta.KubernetesPod) {
	owner, hasOwner := resolveWLMPodOwner(pod)
	if !hasOwner {
		log.Debugf("Ignoring pod %s without owner", pod.ID)
		return
	}

	isSpot := isSpotAssigned(pod)

	log.Debugf("Pod %s added/updated for owner %s (phase=%s, spot=%v)", pod.ID, owner, pod.Phase, isSpot)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Terminal pods are treated as removed to free their spot/on-demand slot
	// before replacement pods are admitted.
	if pod.Phase == string(corev1.PodSucceeded) || pod.Phase == string(corev1.PodFailed) {
		t.deletePodLocked(owner, pod.ID)
		return
	}

	t.getPodsLocked(owner).track(pod.ID, isSpot, podInfo{name: pod.Name, phase: pod.Phase}, t.clock.Now())

	if isSpot {
		// Note: we can not use CreationTimestamp or NodeName of [workloadmeta.KubernetesPod]
		// as they are not populated by comp/core/workloadmeta/collectors/internal/kubeapiserver/pod.go
		// so only check the Phase and use now for createdAt.
		if pod.Phase == string(corev1.PodPending) {
			if _, exists := t.pendingSpotPods[pod.ID]; !exists {
				t.pendingSpotPods[pod.ID] = pendingSpotPod{owner: owner, namespace: pod.Namespace, name: pod.Name, createdAt: t.clock.Now()}
				log.Debugf("Tracking pending spot pod %s", pod.ID)
			}
		} else {
			delete(t.pendingSpotPods, pod.ID)
		}
	}
}

// deleted updates tracking state when a pod is deleted.
func (t *podTracker) deleted(pod *workloadmeta.KubernetesPod) {
	owner, hasOwner := resolveWLMPodOwner(pod)
	if !hasOwner {
		log.Debugf("Ignoring pod %s without owner", pod.ID)
		return
	}

	log.Debugf("Pod %s deleted for owner %s (spot=%v)", pod.ID, owner, isSpotAssigned(pod))

	t.deletePod(owner, pod.ID)
}

// deletePod deletes pod by owner and uid.
func (t *podTracker) deletePod(owner ownerKey, uid string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.deletePodLocked(owner, uid)
}

// deletePodLocked deletes a pod from podsPerOwner. Must be called with t.mu held.
func (t *podTracker) deletePodLocked(owner ownerKey, uid string) {
	if pods, ok := t.podsPerOwner[owner]; ok {
		if pods.delete(uid, t.clock.Now()) {
			delete(t.podsPerOwner, owner)
		}
	}
	delete(t.pendingSpotPods, uid)
}

// getPodsLocked returns the pods for owner, creating it if absent.
// Must be called with t.mu held.
func (t *podTracker) getPodsLocked(owner ownerKey) *pods {
	if pods, ok := t.podsPerOwner[owner]; ok {
		return pods
	}
	pods := t.newPods()
	t.podsPerOwner[owner] = pods
	return pods
}

// getPodToDelete returns the uid, name, and namespace of a pod to delete to make progress toward
// the desired config across all tracked owners, or empty strings if no deletion is needed.
// When a pod is selected, lastUpdate is stamped on its owner's pods to prevent selecting the
// same owner again before the deletion takes effect.
func (t *podTracker) getPodToDelete(rebalanceStabilizationPeriod time.Duration) (string, string, string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.clock.Now()
	lastUpdatedBefore := now.Add(-rebalanceStabilizationPeriod)
	for owner, pods := range t.podsPerOwner {
		if uid, name := pods.getPodToDelete(lastUpdatedBefore); uid != "" {
			pods.lastUpdate = now // suppress re-selection until stabilization period elapses
			return uid, name, owner.Namespace
		}
	}
	return "", "", ""
}

// hasPendingSpotPods returns true if any spot-assigned pod has been pending since before the given time.
func (t *podTracker) hasPendingSpotPods(since time.Time) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, info := range t.pendingSpotPods {
		if info.createdAt.Before(since) {
			return true
		}
	}
	return false
}

// getPendingSpotPods returns all pending spot-assigned pods, keyed by pod UID.
func (t *podTracker) getPendingSpotPods() map[string]pendingSpotPod {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]pendingSpotPod, len(t.pendingSpotPods))
	for uid, info := range t.pendingSpotPods {
		result[uid] = info
	}
	return result
}

// deletePendingSpotPod removes a single pod from pending tracking.
func (t *podTracker) deletePendingSpotPod(uid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pendingSpotPods, uid)
}

func (t *podTracker) newPods() *pods {
	return &pods{
		config:       t.defaultConfig,
		spotUIDs:     make(map[string]podInfo),
		onDemandUIDs: make(map[string]podInfo),
	}
}

// admit increments the in-flight admission count for the given spot/on-demand decision and returns isSpot.
func (p *pods) admit(isSpot bool) bool {
	if isSpot {
		p.admissionSpotCount++
	} else {
		p.admissionOnDemandCount++
	}
	return isSpot
}

// track upserts the pod UID with its info, decrementing the in-flight admission count on first appearance.
func (p *pods) track(uid string, isSpot bool, info podInfo, now time.Time) {
	if isSpot {
		if _, exists := p.spotUIDs[uid]; !exists {
			if p.admissionSpotCount > 0 {
				p.admissionSpotCount--
			}
		}
		p.spotUIDs[uid] = info
	} else {
		if _, exists := p.onDemandUIDs[uid]; !exists {
			if p.admissionOnDemandCount > 0 {
				p.admissionOnDemandCount--
			}
		}
		p.onDemandUIDs[uid] = info
	}
	p.lastUpdate = now
}

// getPodToDelete returns the uid and name of a pod to delete to make progress toward the desired config.
// It returns empty strings if no deletion is needed.
func (p *pods) getPodToDelete(lastUpdatedBefore time.Time) (string, string) {
	if p.admissionSpotCount > 0 || p.admissionOnDemandCount > 0 {
		return "", ""
	}

	if p.lastUpdate.After(lastUpdatedBefore) {
		return "", ""
	}

	if p.hasPending() {
		return "", ""
	}

	spot, onDemand := len(p.spotUIDs), len(p.onDemandUIDs)

	if onDemand < p.config.minOnDemand {
		// minOnDemand not satisfied: remove a spot pod to compensate.
		return pickPod(p.spotUIDs)
	}

	total := spot + onDemand

	desiredSpot := total * p.config.percentage / 100
	if spot > desiredSpot {
		return pickPod(p.spotUIDs)
	}

	desiredOnDemand := max(total-desiredSpot, p.config.minOnDemand)
	if onDemand > desiredOnDemand {
		return pickPod(p.onDemandUIDs)
	}

	return "", ""
}

// hasPending returns true if any tracked pod is in PodPending phase.
func (p *pods) hasPending() bool {
	const pending = string(corev1.PodPending)
	for _, info := range p.spotUIDs {
		if info.phase == pending {
			return true
		}
	}
	for _, info := range p.onDemandUIDs {
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
func (p *pods) delete(uid string, now time.Time) bool {
	delete(p.spotUIDs, uid)
	delete(p.onDemandUIDs, uid)
	p.lastUpdate = now
	return len(p.spotUIDs) == 0 && len(p.onDemandUIDs) == 0 && p.admissionSpotCount == 0 && p.admissionOnDemandCount == 0
}

// totalCount returns the total number of pods including in-flight admissions.
func (p *pods) totalCount() int {
	return len(p.spotUIDs) + len(p.onDemandUIDs) + p.admissionSpotCount + p.admissionOnDemandCount
}

// spotCount returns the number of spot-assigned pods including in-flight spot admissions.
func (p *pods) spotCount() int {
	return len(p.spotUIDs) + p.admissionSpotCount
}

func isSpotAssigned(pod *workloadmeta.KubernetesPod) bool {
	return pod.Labels[SpotAssignedLabel] == SpotAssignedSpot
}
