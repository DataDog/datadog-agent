// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PodWatcher regularly pools the kubelet for new/changed/removed containers.
// It keeps an internal state to only send the updated pods.
type PodWatcher struct {
	sync.Mutex
	kubeUtil       *KubeUtil
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
	readinessCache map[string]bool
	lastSeenReady  map[string]time.Time
	tagsDigest     map[string]string
}

// NewPodWatcher creates a new watcher. User call must then trigger PullChanges
// and ExpireContainers when needed.
func NewPodWatcher(expiryDuration time.Duration) (*PodWatcher, error) {
	kubeutil, err := GetKubeUtil()
	if err != nil {
		return nil, err
	}
	watcher := &PodWatcher{
		kubeUtil:       kubeutil,
		lastSeen:       make(map[string]time.Time),
		readinessCache: make(map[string]bool),
		lastSeenReady:  make(map[string]time.Time),
		tagsDigest:     make(map[string]string),
		expiryDuration: expiryDuration,
	}
	return watcher, nil
}

// PullChanges pulls a new podList from the kubelet and returns Pod objects for
// new / updated pods. Updated pods will be sent entirely, user must replace
// previous info for these pods.
func (w *PodWatcher) PullChanges() ([]*Pod, error) {
	var podList []*Pod
	podList, err := w.kubeUtil.GetLocalPodList()
	if err != nil {
		return podList, err
	}
	return w.computeChanges(podList)
}

// computeChanges is used by PullChanges, split for testing
func (w *PodWatcher) computeChanges(podList []*Pod) ([]*Pod, error) {
	now := time.Now()
	var updatedPods []*Pod

	w.Lock()
	defer w.Unlock()
	for _, pod := range podList {
		podEntity := PodUIDToEntityName(pod.Metadata.UID)
		newStaticPod := false
		_, foundPod := w.lastSeen[podEntity]

		if !foundPod {
			w.tagsDigest[podEntity] = digestPodMeta(pod.Metadata)
		}

		// static pods are included specifically because they won't have any container
		// as they're not updated in the pod list after creation
		if isPodStatic(pod) && !foundPod {
			newStaticPod = true
		}

		// Refresh last pod seen time
		w.lastSeen[podEntity] = now

		// Detect updated containers
		updatedContainer := false
		isPodReady := IsPodReady(pod)

		for _, container := range pod.Status.GetAllContainers() {
			// We don't check container readiness as init containers are never ready
			// We check if the container has an ID instead (has run or is running)
			if !container.IsPending() {
				// new container are always sent ignoring the pod state
				if _, found := w.lastSeen[container.ID]; !found {
					updatedContainer = true
				}
				w.lastSeen[container.ID] = now

				// for existing ones we look at the previous pod state
				wasPodReady, found := w.readinessCache[container.ID]
				if found && isPodReady && !wasPodReady {
					// if pod became ready we want to update it
					updatedContainer = true
				}
				w.readinessCache[container.ID] = isPodReady

				// update the readiness expiry cache
				if isPodReady {
					w.lastSeenReady[container.ID] = now
				}
			}
		}

		// Detect changes in labels and annotations values
		newLabelsOrAnnotations := false
		newTagsDigest := digestPodMeta(pod.Metadata)
		if foundPod && newTagsDigest != w.tagsDigest[podEntity] {
			// update tags digest
			w.tagsDigest[podEntity] = newTagsDigest
			newLabelsOrAnnotations = true
		}

		if newStaticPod || updatedContainer || newLabelsOrAnnotations {
			updatedPods = append(updatedPods, pod)
		}
	}
	log.Debugf("Found %d changed pods out of %d", len(updatedPods), len(podList))
	return updatedPods, nil
}

// Expire returns a list of entities (containers and pods)
// that are not listed in the podlist anymore. It must be called
// immediately after a PullChanges.
// For containers, string is kubernetes container ID (with runtime name)
// For pods, string is "kubernetes_pod://uid" format
func (w *PodWatcher) Expire() ([]string, error) {
	now := time.Now()
	w.Lock()
	defer w.Unlock()
	var expiredContainers []string

	for id, lastSeen := range w.lastSeen {
		// pod was removed from the pod list, we can safely cleanup everything
		if now.Sub(lastSeen) > w.expiryDuration {
			delete(w.lastSeen, id)
			delete(w.tagsDigest, id)
			delete(w.readinessCache, id)
			delete(w.lastSeenReady, id)
			expiredContainers = append(expiredContainers, id)
		}
	}
	for id, lastSeenReady := range w.lastSeenReady {
		// we keep pods gone unready for 30 seconds and then force removal
		if now.Sub(lastSeenReady) > 30*time.Second {
			delete(w.lastSeenReady, id)
			expiredContainers = append(expiredContainers, id)
		}
	}

	return expiredContainers, nil
}

// GetPodForEntityID finds the pod corresponding to an entity.
// EntityIDs can be Docker container IDs or pod UIDs (prefixed).
// Returns a nil pointer if not found.
func (w *PodWatcher) GetPodForEntityID(entityID string) (*Pod, error) {
	return w.kubeUtil.GetPodForEntityID(entityID)
}

// digestPodMeta returns a unique hash of pod labels
// and annotations.
// it hashes labels then annotations and makes a single hash of both maps
func digestPodMeta(meta PodMetadata) string {
	h := fnv.New64()
	h.Write([]byte(digestMapValues(meta.Labels)))
	h.Write([]byte(digestMapValues(meta.Annotations)))
	return strconv.FormatUint(h.Sum64(), 16)
}

// digestMapValues returns a unique hash of map values
// used to track changes in labels and annotations values
// it takes into consideration the random keys order in a map
// by hashing the values after sorting the keys
func digestMapValues(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}

	// to store the keys in slice in sorted order
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := fnv.New64()
	for _, k := range keys {
		h.Write([]byte(m[k]))
	}
	return strconv.FormatUint(h.Sum64(), 16)
}
