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
	createdAt time.Time
}

// pods tracks spot and on-demand pods and in-flight admission counts for the same owner.
type pods struct {
	spotUIDs               map[string]struct{}
	onDemandUIDs           map[string]struct{}
	admissionSpotCount     int
	admissionOnDemandCount int
}

// podTracker keeps track of pods per owner.
type podTracker struct {
	mu    sync.RWMutex
	clock clock.Clock
	// podsPerOwner groups pods and in-flight admission counts by owner.
	podsPerOwner map[ownerKey]*pods
	// pendingSpotPods tracks spot-assigned pods that are pending scheduling, keyed by pod UID.
	pendingSpotPods map[string]pendingSpotPod
}

func newPodTracker(clk clock.Clock) *podTracker {
	return &podTracker{
		clock:           clk,
		podsPerOwner:    make(map[ownerKey]*pods),
		pendingSpotPods: make(map[string]pendingSpotPod),
	}
}

// admitNewPod reads the current pod counts for owner and calls decideSpot(total, spot)
// to determine pod assignment.
// decideSpot receives the total number and the number of existing spot-assigned pods and
// should return true if pod will be assigned to spot instance.
// admitNewPod locks podTracker state for the duration of decideSpot call therefore it must be fast.
// It returns the decideSpot result.
func (t *podTracker) admitNewPod(owner ownerKey, decideSpot func(total, spot int) bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	pods := t.getPodsLocked(owner)

	isSpot := decideSpot(pods.totalCount(), pods.spotCount())

	pods.admit(isSpot)

	return isSpot
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

	t.getPodsLocked(owner).track(pod.ID, isSpot)

	if isSpot {
		// Note: we can not use CreationTimestamp or NodeName of [workloadmeta.KubernetesPod]
		// as they are not populated by comp/core/workloadmeta/collectors/internal/kubeapiserver/pod.go
		// so only check the Phase and use now for createdAt.
		if pod.Phase == string(corev1.PodPending) {
			if _, exists := t.pendingSpotPods[pod.ID]; !exists {
				t.pendingSpotPods[pod.ID] = pendingSpotPod{owner: owner, createdAt: t.clock.Now()}
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
		if pods.delete(uid) {
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
	pods := newPods()
	t.podsPerOwner[owner] = pods
	return pods
}

// getPendingSpotPods returns spot-assigned pods created before since, grouped by rollout owner.
func (t *podTracker) getPendingSpotPods(since time.Time) map[ownerKey][]string {
	pendingSince := map[string]pendingSpotPod{}
	t.mu.RLock()
	for uid, info := range t.pendingSpotPods {
		if info.createdAt.Before(since) {
			pendingSince[uid] = info
		}
	}
	t.mu.RUnlock()

	result := map[ownerKey][]string{}
	for uid, info := range pendingSince {
		rolloutOwner, ok := resolveRolloutOwner(info.owner)
		if !ok {
			log.Warnf("Cannot resolve rollout owner for %s, skipping", info.owner)
			continue
		}
		result[rolloutOwner] = append(result[rolloutOwner], uid)
	}
	return result
}

// removePendingSpotPods removes the given pods from pending tracking.
func (t *podTracker) removePendingSpotPods(uids []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, uid := range uids {
		delete(t.pendingSpotPods, uid)
	}
}

func newPods() *pods {
	return &pods{
		spotUIDs:     make(map[string]struct{}),
		onDemandUIDs: make(map[string]struct{}),
	}
}

// admit increments the in-flight admission count for the given spot/on-demand decision.
func (p *pods) admit(isSpot bool) {
	if isSpot {
		p.admissionSpotCount++
	} else {
		p.admissionOnDemandCount++
	}
}

// track upserts the pod UID and decrements the corresponding in-flight admission count on first appearance.
func (p *pods) track(uid string, isSpot bool) {
	if isSpot {
		if _, exists := p.spotUIDs[uid]; !exists {
			p.spotUIDs[uid] = struct{}{}
			if p.admissionSpotCount > 0 {
				p.admissionSpotCount--
			}
		}
	} else {
		if _, exists := p.onDemandUIDs[uid]; !exists {
			p.onDemandUIDs[uid] = struct{}{}
			if p.admissionOnDemandCount > 0 {
				p.admissionOnDemandCount--
			}
		}
	}
}

// delete removes pod by uid and returns true if it tracks no more pods including in-flight admissions.
func (p *pods) delete(uid string) bool {
	delete(p.spotUIDs, uid)
	delete(p.onDemandUIDs, uid)
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
