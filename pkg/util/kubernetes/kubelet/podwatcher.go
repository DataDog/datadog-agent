// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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

const unreadinessTimeout = 30 * time.Second

// PodWatcher regularly pools the kubelet for new/changed/removed containers.
// It keeps an internal state to only send the updated pods.
type PodWatcher struct {
	sync.Mutex
	kubeUtil       KubeUtilInterface
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
	lastSeenReady  map[string]time.Time
	tagsDigest     map[string]string
}

// NewPodWatcher creates a new watcher given an expiry duration
// and if the watcher should watch label/annotation changes on pods.
// User call must then trigger PullChanges and Expire when needed.
func NewPodWatcher(expiryDuration time.Duration, isWatchingTags bool) (*PodWatcher, error) {
	kubeutil, err := GetKubeUtil()
	if err != nil {
		return nil, err
	}
	watcher := &PodWatcher{
		kubeUtil:       kubeutil,
		lastSeen:       make(map[string]time.Time),
		lastSeenReady:  make(map[string]time.Time),
		expiryDuration: expiryDuration,
	}
	if isWatchingTags {
		watcher.tagsDigest = make(map[string]string)
	}
	return watcher, nil
}

// isWatchingTags returns true if the pod watcher should
// watch for tag changes on pods
func (w *PodWatcher) isWatchingTags() bool {
	return w.tagsDigest != nil
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

		if w.isWatchingTags() && !foundPod {
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

				// for existing ones we look at the readiness state
				if _, found := w.lastSeenReady[container.ID]; !found && isPodReady {
					// the pod has never been seen ready or was removed when
					// reaching the unreadinessTimeout
					updatedContainer = true
				}

				// update the readiness expiry cache
				if isPodReady {
					w.lastSeenReady[container.ID] = now
				}
			}
		}

		// Detect changes in labels and annotations values
		newLabelsOrAnnotations := false
		if w.isWatchingTags() {
			newTagsDigest := digestPodMeta(pod.Metadata)
			if foundPod && newTagsDigest != w.tagsDigest[podEntity] {
				// update tags digest
				w.tagsDigest[podEntity] = newTagsDigest
				newLabelsOrAnnotations = true
			}
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
			delete(w.lastSeenReady, id)
			if w.isWatchingTags() {
				delete(w.tagsDigest, id)
			}
			expiredContainers = append(expiredContainers, id)
		}
	}
	for id, lastSeenReady := range w.lastSeenReady {
		// we keep pods gone unready for 25 seconds and then force removal
		if now.Sub(lastSeenReady) > unreadinessTimeout {
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
	h.Write([]byte(digestMapValues(meta.Labels)))      //nolint:errcheck
	h.Write([]byte(digestMapValues(meta.Annotations))) //nolint:errcheck
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
		h.Write([]byte(m[k])) //nolint:errcheck
	}
	return strconv.FormatUint(h.Sum64(), 16)
}
