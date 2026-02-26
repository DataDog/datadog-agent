// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"sync"
	"time"

	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type pendingSpotPod struct {
	owner     ownerKey
	createdAt time.Time
}

// podTracker keeps track of pods per owner.
type podTracker struct {
	mu    sync.RWMutex
	clock clock.Clock
	// podsPerOwner indexes running pods by owner
	podsPerOwner map[ownerKey]map[string]*workloadmeta.KubernetesPod
	// admissionSpotCount and admissionOnDemandCount track pods whose spot/on-demand decision
	// was recorded at admission time but have not yet appeared in podsPerOwner
	admissionSpotCount     map[ownerKey]int
	admissionOnDemandCount map[ownerKey]int
	// pendingSpotPods tracks spot-assigned pods that are pending scheduling, keyed by pod UID.
	pendingSpotPods map[string]pendingSpotPod
}

func newPodTracker(clk clock.Clock) *podTracker {
	return &podTracker{
		clock:                  clk,
		podsPerOwner:           make(map[ownerKey]map[string]*workloadmeta.KubernetesPod),
		admissionSpotCount:     make(map[ownerKey]int),
		admissionOnDemandCount: make(map[ownerKey]int),
		pendingSpotPods:        make(map[string]pendingSpotPod),
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

	pods := t.podsPerOwner[owner]
	existingPods := len(pods) + t.admissionSpotCount[owner] + t.admissionOnDemandCount[owner]
	existingSpot := countSpotAssigned(pods) + t.admissionSpotCount[owner]

	isSpot := decideSpot(existingPods, existingSpot)
	if isSpot {
		t.admissionSpotCount[owner]++
	} else {
		t.admissionOnDemandCount[owner]++
	}
	return isSpot
}

// addedOrUpdated updates tracking state when a pod is added or updated.
func (t *podTracker) addedOrUpdated(pod *workloadmeta.KubernetesPod) {
	owner, hasOwner := resolveWLMPodOwner(pod)
	if !hasOwner {
		log.Debugf("Ignoring pod %s without owner", pod.ID)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	seenBefore := false
	if ownerPods, exists := t.podsPerOwner[owner]; exists {
		_, seenBefore = ownerPods[pod.ID]
	} else {
		t.podsPerOwner[owner] = make(map[string]*workloadmeta.KubernetesPod)
	}
	t.podsPerOwner[owner][pod.ID] = pod

	if isSpotAssigned(pod) {
		// Note: we can not use CreationTimestamp or NodeName of [workloadmeta.KubernetesPod]
		// as they are not populated by comp/core/workloadmeta/collectors/internal/kubeapiserver/pod.go
		// so only check the Phase and use now for createdAt.
		if pod.Phase == "Pending" {
			if _, exists := t.pendingSpotPods[pod.ID]; !exists {
				t.pendingSpotPods[pod.ID] = pendingSpotPod{owner: owner, createdAt: t.clock.Now()}
				log.Debugf("Tracking pending spot pod %s", pod.ID)
			}
		} else {
			delete(t.pendingSpotPods, pod.ID)
		}

		if !seenBefore && t.admissionSpotCount[owner] > 0 {
			t.admissionSpotCount[owner]--
		}
	} else {
		if !seenBefore && t.admissionOnDemandCount[owner] > 0 {
			t.admissionOnDemandCount[owner]--
		}
	}
}

// removed updates tracking state when a pod is removed.
func (t *podTracker) removed(pod *workloadmeta.KubernetesPod) {
	owner, hasOwner := resolveWLMPodOwner(pod)
	if !hasOwner {
		log.Debugf("Ignoring pod %s without owner", pod.ID)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if ownerPods, exists := t.podsPerOwner[owner]; exists {
		if len(ownerPods) == 1 {
			delete(t.podsPerOwner, owner)
		} else {
			delete(ownerPods, pod.ID)
		}
	}
	delete(t.pendingSpotPods, pod.ID)
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

func countSpotAssigned(pods map[string]*workloadmeta.KubernetesPod) int {
	spotAssigned := 0
	for _, pods := range pods {
		if isSpotAssigned(pods) {
			spotAssigned++
		}
	}
	return spotAssigned
}

func isSpotAssigned(pod *workloadmeta.KubernetesPod) bool {
	return pod.Labels[SpotAssignedLabel] == "true"
}
