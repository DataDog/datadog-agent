// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// How long should we wait before scanning for new pods.
	// This can't be too low to avoid putting too much pressure on the kubelet.
	// Also, currently KubeUtil has an internal cache duration, there is no
	// point setting the period lower than that.
	podScanPeriod = 10 * time.Second

	// How should we wait before considering a pod has been deleted.
	podExpiration = 20 * time.Second
)

// KubeletWatcher looks for new and deleted pods polling the kubelet.
type KubeletWatcher struct {
	watcher *kubelet.PodWatcher
	added   chan *kubelet.Pod
	removed chan *kubelet.Pod
	stopped chan struct{}
}

// NewKubeletWatcher returns a new watcher.
func NewKubeletWatcher() (*KubeletWatcher, error) {
	watcher, err := kubelet.NewPodWatcher(podExpiration)
	if err != nil {
		return nil, err
	}
	return &KubeletWatcher{
		watcher: watcher,
		added:   make(chan *kubelet.Pod),
		removed: make(chan *kubelet.Pod),
	}, nil
}

// Start starts the watcher.
func (w *KubeletWatcher) Start() {
	go w.run()
}

// Stop stops the watcher.
func (w *KubeletWatcher) Stop() {
	w.stopped <- struct{}{}
}

// Added returns a channel of new pods.
func (w *KubeletWatcher) Added() chan *kubelet.Pod {
	return w.added
}

// Removed returns a channel of pods removed.
func (w *KubeletWatcher) Removed() chan *kubelet.Pod {
	return w.removed
}

// run runs periodically a scan to detect new and deleted pod.
func (w *KubeletWatcher) run() {
	ticker := time.NewTicker(podScanPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.addPods()
			w.removePods()
		case <-w.stopped:
			return
		}
	}
}

// addPods pulls the new pods and passes them along to the channel.
func (w *KubeletWatcher) addPods() {
	pods, err := w.watcher.PullChanges()
	if err != nil {
		log.Error("can't list changed pods", err)
		return
	}
	for _, pod := range pods {
		if pod.Status.Phase == "Running" {
			w.added <- pod
		}
	}
}

// removePods fetches all expired pods and passes them along to the channel.
func (w *KubeletWatcher) removePods() {
	entityIDs, err := w.watcher.Expire()
	if err != nil {
		log.Errorf("can't list expired pods: %v", err)
		return
	}
	for _, entityID := range entityIDs {
		pod, err := w.watcher.GetPodForEntityID(entityID)
		if err != nil {
			log.Errorf("can't find pod %v: %v", entityID, err)
			continue
		}
		w.removed <- pod
	}
}
