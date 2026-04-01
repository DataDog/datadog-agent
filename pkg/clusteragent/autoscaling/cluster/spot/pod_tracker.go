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
	workload  workload
	name      string
	createdAt time.Time
}

// pods tracks spot and on-demand pods and in-flight admission counts for the same owner.
type pods struct {
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
// Pods are grouped first by workload (e.g. Deployment) and then by direct owner
// (e.g. ReplicaSet). This enables O(1) per-workload operations.
type podTracker struct {
	clock         clock.Clock
	defaultConfig workloadSpotConfig
	configSource  func(workload) (workloadSpotConfig, bool)

	mu              sync.RWMutex
	podGroups       map[workload]map[podOwner]*pods
	pendingSpotPods map[string]pendingSpotPod
}

func newPodTracker(clk clock.Clock, defaultConfig workloadSpotConfig, configSource func(workload) (workloadSpotConfig, bool)) *podTracker {
	return &podTracker{
		clock:           clk,
		defaultConfig:   defaultConfig,
		configSource:    configSource,
		podGroups:       make(map[workload]map[podOwner]*pods),
		pendingSpotPods: make(map[string]pendingSpotPod),
	}
}

// admitNewPod decides whether the new pod should be spot-assigned using
// the per-group config and returns true if the pod was assigned to spot.
func (t *podTracker) admitNewPod(g podGroup) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	pods := t.getOrCreatePodsLocked(g)
	t.refreshConfigLocked(g.workload, pods)

	total := pods.totalCount()
	spot := pods.spotCount()
	onDemand := total - spot

	if onDemand < pods.config.minOnDemand {
		log.Debugf("Skipping pod for %s: on-demand minimum not met (%d < %d), total: %d, spot: %d", g.owner, onDemand, pods.config.minOnDemand, total, spot)
		return pods.admit(false)
	}

	desiredSpot := (total + 1) * pods.config.percentage / 100
	if spot >= desiredSpot {
		log.Debugf("Skipping pod for %s: desired spot reached (%d >= %d), total: %d", g.owner, spot, desiredSpot, total)
		return pods.admit(false)
	}

	log.Debugf("Assigning pod for %s to spot (%d of desired %d spot, %d on-demand), total: %d", g.owner, spot, desiredSpot, onDemand, total)
	return pods.admit(true)
}

// admitNewOnDemandPod records an on-demand admission for the pod group.
func (t *podTracker) admitNewOnDemandPod(g podGroup) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.getOrCreatePodsLocked(g).admit(false)
}

// addedOrUpdated updates tracking state when a pod is added or updated.
func (t *podTracker) addedOrUpdated(pod *workloadmeta.KubernetesPod) {
	g, ok := resolveWLMPodGroup(pod)
	if !ok {
		log.Debugf("Ignoring pod %s: cannot resolve group", pod.ID)
		return
	}

	isSpot := isSpotAssigned(pod)

	log.Debugf("Pod %s added/updated for owner %s (phase=%s, spot=%v)", pod.ID, g.owner, pod.Phase, isSpot)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Terminal pods are treated as removed to free their spot/on-demand slot
	// before replacement pods are admitted.
	if pod.Phase == string(corev1.PodSucceeded) || pod.Phase == string(corev1.PodFailed) {
		t.deletePodLocked(g, pod.ID)
		return
	}

	t.getOrCreatePodsLocked(g).track(pod.ID, isSpot, podInfo{name: pod.Name, phase: pod.Phase}, t.clock.Now())

	if isSpot {
		// Note: we can not use CreationTimestamp or NodeName of [workloadmeta.KubernetesPod]
		// as they are not populated by comp/core/workloadmeta/collectors/internal/kubeapiserver/pod.go
		// so only check the Phase and use now for createdAt.
		if pod.Phase == string(corev1.PodPending) {
			if _, exists := t.pendingSpotPods[pod.ID]; !exists {
				t.pendingSpotPods[pod.ID] = pendingSpotPod{workload: g.workload, name: pod.Name, createdAt: t.clock.Now()}
				log.Debugf("Tracking pending spot pod %s", pod.ID)
			}
		} else {
			delete(t.pendingSpotPods, pod.ID)
		}
	}
}

// deleted updates tracking state when a pod is deleted.
func (t *podTracker) deleted(pod *workloadmeta.KubernetesPod) {
	g, ok := resolveWLMPodGroup(pod)
	if !ok {
		log.Debugf("Ignoring pod %s: cannot resolve group", pod.ID)
		return
	}

	log.Debugf("Pod %s deleted for owner %s (spot=%v)", pod.ID, g.owner, isSpotAssigned(pod))

	t.deletePod(g, pod.ID)
}

// deletePod deletes pod by group and uid.
func (t *podTracker) deletePod(g podGroup, uid string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.deletePodLocked(g, uid)
}

// deletePodLocked deletes a pod from podGroups. Must be called with t.mu held.
func (t *podTracker) deletePodLocked(g podGroup, uid string) {
	if owners, ok := t.podGroups[g.workload]; ok {
		if pods, ok := owners[g.owner]; ok {
			if pods.delete(uid, t.clock.Now()) {
				delete(owners, g.owner)
			}
		}
		if len(owners) == 0 {
			delete(t.podGroups, g.workload)
		}
	}
	delete(t.pendingSpotPods, uid)
}

// refreshConfigLocked refreshes the spot config for pods from the configSource.
// Must be called with t.mu held.
func (t *podTracker) refreshConfigLocked(w workload, pods *pods) {
	if cfg, ok := t.configSource(w); ok {
		pods.config = cfg
	}
}

// getOrCreatePodsLocked returns the pods for g, creating it if absent.
// Must be called with t.mu held.
func (t *podTracker) getOrCreatePodsLocked(g podGroup) *pods {
	owners, ok := t.podGroups[g.workload]
	if !ok {
		owners = make(map[podOwner]*pods)
		t.podGroups[g.workload] = owners
	}
	if pods, ok := owners[g.owner]; ok {
		return pods
	}
	pods := t.newPods()
	owners[g.owner] = pods
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
	for w, owners := range t.podGroups {
		for owner, pods := range owners {
			t.refreshConfigLocked(w, pods)
			if pods.config.isDisabled(now) {
				continue
			}
			if uid, name := pods.getPodToDelete(lastUpdatedBefore); uid != "" {
				pods.lastUpdate = now // suppress re-selection until stabilization period elapses
				return uid, name, owner.Namespace
			}
		}
	}
	return "", "", ""
}

// getPendingSpotPods returns spot-assigned pods that have been pending since before the given time keyed by pod UID.
func (t *podTracker) getPendingSpotPods(since time.Time) map[string]pendingSpotPod {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]pendingSpotPod)
	for uid, info := range t.pendingSpotPods {
		if info.createdAt.Before(since) {
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
