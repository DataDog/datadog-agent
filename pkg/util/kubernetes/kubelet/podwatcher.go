// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
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
	oldPhase       map[string]string
	oldReadiness   map[string]bool
}

// NewPodWatcher creates a new watcher given an expiry duration
// and if the watcher should watch label/annotation changes on pods.
// User call must then trigger PullChanges and Expire when needed.
func NewPodWatcher(expiryDuration time.Duration) (*PodWatcher, error) {
	kubeutil, err := GetKubeUtil()
	if err != nil {
		return nil, err
	}
	watcher := &PodWatcher{
		kubeUtil:       kubeutil,
		lastSeen:       make(map[string]time.Time),
		lastSeenReady:  make(map[string]time.Time),
		tagsDigest:     make(map[string]string),
		oldPhase:       make(map[string]string),
		oldReadiness:   make(map[string]bool),
		expiryDuration: expiryDuration,
	}
	return watcher, nil
}

// PullChanges pulls a new podList from the kubelet and returns Pod objects for
// new / updated pods. Updated pods will be sent entirely, user must replace
// previous info for these pods.
func (w *PodWatcher) PullChanges(ctx context.Context) ([]*Pod, error) {
	var podList []*Pod
	podList, err := w.kubeUtil.GetLocalPodList(ctx)
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
		newPod := false

		_, foundPod := w.lastSeen[podEntity]
		if !foundPod {
			w.tagsDigest[podEntity] = digestPodMeta(pod.Metadata)
			w.oldPhase[podEntity] = pod.Status.Phase
			newPod = true
		}

		// Refresh last pod seen time
		w.lastSeen[podEntity] = now

		// Detect updated containers
		updatedContainer := false
		isPodReady := IsPodReady(pod)

		for _, container := range pod.Status.GetAllContainers() {
			if container.IsPending() {
				// We don't check container readiness as init
				// containers are never ready. We check if the
				// container has an ID instead (has run or is
				// running)
				continue
			}

			// new container are always sent ignoring the pod state
			if _, found := w.lastSeen[container.ID]; !found {
				updatedContainer = true
			}
			w.lastSeen[container.ID] = now

			// for existing containers, check whether the
			// readiness has changed since last time
			if oldReadiness, found := w.oldReadiness[container.ID]; !found || oldReadiness != isPodReady {
				// the pod has never been seen ready or was removed when
				// reaching the unreadinessTimeout
				updatedContainer = true
			}

			w.oldReadiness[container.ID] = isPodReady

			if isPodReady {
				w.lastSeenReady[container.ID] = now
			}
		}

		newLabelsOrAnnotations := false
		newPhase := false
		newTagsDigest := digestPodMeta(pod.Metadata)

		// if the pod already existed, check whether tagsDigest or
		// phase changed
		if foundPod {
			if newTagsDigest != w.tagsDigest[podEntity] {
				w.tagsDigest[podEntity] = newTagsDigest
				newLabelsOrAnnotations = true
			}

			if pod.Status.Phase != w.oldPhase[podEntity] {
				w.oldPhase[podEntity] = pod.Status.Phase
				newPhase = true
			}
		}

		if newPod || updatedContainer || newLabelsOrAnnotations || newPhase {
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
			delete(w.tagsDigest, id)
			delete(w.oldPhase, id)
			delete(w.oldReadiness, id)
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
