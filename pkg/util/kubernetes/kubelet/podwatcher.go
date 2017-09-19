// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package kubelet

import (
	"strconv"
	"sync"
	"time"

	log "github.com/cihub/seelog"
)

// PodWatcher regularly pools the kubelet for new/changed/removed containers.
// It keeps an internal state to only send the updated pods.
type PodWatcher struct {
	sync.Mutex
	kubeUtil         *KubeUtil
	latestResVersion int
	expiryDuration   time.Duration
	lastSeen         map[string]time.Time
}

// NewPodWatcher creates a new watcher. User call must then trigger PullChanges
// and ExpireContainers when needed.
func NewPodWatcher() (*PodWatcher, error) {
	kubeutil, err := NewKubeUtil()
	if err != nil {
		return nil, err
	}
	watcher := &PodWatcher{
		kubeUtil:         kubeutil,
		latestResVersion: -1,
		lastSeen:         make(map[string]time.Time),
		expiryDuration:   5 * time.Minute,
	}
	return watcher, nil
}

// PullChanges pulls a new podlist from the kubelet and returns Pod objects for
// new / updated pods. Updated pods will be sent entierly, user must replace
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
	newResVersion := w.latestResVersion
	var updatedPods []*Pod

	w.Lock()
	defer w.Unlock()
	for _, pod := range podlist {
		// Converting resVersion
		var version int
		if pod.Metadata.ResVersion == "" {
			/* System pods don't have a ressource version, using
			   0 to return them once. If they're restarted, we
			   will detect the new containers and send the pod
			   again anyway */
			version = 0
		} else {
			var err error
			version, err = strconv.Atoi(pod.Metadata.ResVersion)
			if err != nil {
				log.Warnf("can't parse resVersion %s for pod %s: %s",
					pod.Metadata.ResVersion, pod.Metadata.Name, err)
			}
		}
		// Detect new/updated pods
		newPod := false
		if version > w.latestResVersion {
			newPod = true
			if version > newResVersion {
				newResVersion = version
			}
		}
		// Detect new containers within existing pods
		newContainer := false
		for _, container := range pod.Status.Containers {
			if _, found := w.lastSeen[container.ID]; found == false {
				newContainer = true
			}
			w.lastSeen[container.ID] = now
		}
		if newPod || newContainer {
			updatedPods = append(updatedPods, pod)
		}
	}
	log.Debugf("found %d changed pods out of %d, new resversion %d",
		len(updatedPods), len(podlist), newResVersion)
	w.latestResVersion = newResVersion
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
