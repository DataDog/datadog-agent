// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"sync"
	"time"

	log "github.com/cihub/seelog"
)

// PodWatcher regularly pools the kubelet for new/changed/removed containers.
// It keeps an internal state to only send the updated pods.
type PodWatcher struct {
	sync.Mutex
	kubeUtil       *KubeUtil
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
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
		expiryDuration: expiryDuration,
	}
	return watcher, nil
}

// PullChanges pulls a new podlist from the kubelet and returns Pod objects for
// new / updated pods. Updated pods will be sent entirely, user must replace
// previous info for these pods.
func (w *PodWatcher) PullChanges() ([]*Pod, error) {
	podlist, err := w.kubeUtil.GetLocalPodList()
	if err != nil {
		return []*Pod{}, err
	}
	return w.computechanges(podlist)
}

// computechanges is used by PullChanges, split for testing
func (w *PodWatcher) computechanges(podlist []*Pod) ([]*Pod, error) {
	now := time.Now()
	var updatedPods []*Pod

	w.Lock()
	defer w.Unlock()
	for _, pod := range podlist {
		// Detect new containers
		newContainer := false
		for _, container := range pod.Status.Containers {
			if _, found := w.lastSeen[container.ID]; found == false {
				newContainer = true
			}
			w.lastSeen[container.ID] = now
		}
		if newContainer {
			updatedPods = append(updatedPods, pod)
		}
	}
	log.Debugf("found %d changed pods out of %d",
		len(updatedPods), len(podlist))
	return updatedPods, nil
}

// ExpireContainers returns a list of container id for containers
// that are not listed in the podlist anymore. It must be called
// immediately after a PullChanges.
func (w *PodWatcher) ExpireContainers() ([]string, error) {
	now := time.Now()
	w.Lock()
	defer w.Unlock()
	var expiredContainers []string

	for id, lastSeen := range w.lastSeen {
		if now.Sub(lastSeen) > w.expiryDuration {
			expiredContainers = append(expiredContainers, id)
		}
	}
	if len(expiredContainers) > 0 {
		for _, id := range expiredContainers {
			delete(w.lastSeen, id)
		}
	}
	return expiredContainers, nil
}

// GetPodForContainerID fetches the podlist and returns the pod running
// a given container on the node. Returns a nil pointer if not found.
// It just proxies the call to its kubeutil.
func (w *PodWatcher) GetPodForContainerID(containerID string) (*Pod, error) {
	return w.kubeUtil.GetPodForContainerID(containerID)
}
