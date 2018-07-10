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

// Kubelet looks for new and deleted pods polling the kubelet.
type Kubelet struct {
	watcher *kubelet.PodWatcher
	added   chan *kubelet.Pod
	removed chan *kubelet.Pod
	stopped chan struct{}
}

// NewKubelet returns a new watcher.
func NewKubelet(added, removed chan *kubelet.Pod) (*Kubelet, error) {
	watcher, err := kubelet.NewPodWatcher(podExpiration)
	if err != nil {
		return nil, err
	}
	return &Kubelet{
		watcher: watcher,
		added:   added,
		removed: removed,
	}, nil
}

// Start starts the watcher.
func (w *Kubelet) Start() {
	go w.run()
}

// Stop stops the watcher.
func (w *Kubelet) Stop() {
	w.stopped <- struct{}{}
}

// run runs periodically a scan to detect new and deleted pod.
func (w *Kubelet) run() {
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
func (w *Kubelet) addPods() {
	pods, err := w.watcher.PullChanges()
	if err != nil {
		log.Warnf("Can't list changed pods: %v", err)
		return
	}
	for _, pod := range pods {
		w.added <- pod
	}
}

// removePods fetches all expired pods and passes them along to the channel.
func (w *Kubelet) removePods() {
	entityIDs, err := w.watcher.Expire()
	if err != nil {
		log.Warnf("Can't list expired pods: %v", err)
		return
	}
	for _, entityID := range entityIDs {
		pod, err := w.watcher.GetPodForEntityID(entityID)
		if err != nil {
			log.Warnf("Can't find pod %v: %v", entityID, err)
			continue
		}
		w.removed <- pod
	}
}
