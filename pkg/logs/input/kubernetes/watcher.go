// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// build kubelet

package container

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Watcher provides the new pods and the ones that have been removed.
type Watcher interface {
	Start()
	Stop()
	Added() chan *kubelet.Pod
	Removed() chan *kubelet.Pod
}

// Strategy represents the strategy to collect new and removed pods.
type Strategy uint32

const (
	KubeletPolling Strategy = 1 << iota
	Inotify
)

// NewWatcher returns a new watcher.
func NewWatcher(strategy Strategy) (Watcher, error) {
	switch strategy {
	case KubeletPolling:
		return KubeletWatcher()
	case Inotify:
		return NewFileSystemWatcher()
	}
}

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
		case <-s.stopped:
			return
		}
	}
}

// addPods pulls the new pods and passes them along to the channel.
func (w *KubeletWatcher) addPods() {
	pods, err := s.watcher.PullChanges()
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
	entityIDs, err := s.watcher.Expire()
	if err != nil {
		log.Errorf("can't list expired pods: %v", err)
		return
	}
	for _, entityID := range entityIDs {
		pod, err := s.watcher.GetPodForEntityID(entityID)
		if err != nil {
			log.Errorf("can't find pod %v: %v", entityID, err)
			continue
		}
		w.removed <- pod
	}
}

// FileSystemWatcher looks for new and deleted pods listening to file system events.
type FileSystemWatcher struct {
	watcher  *fsnotify.Watcher
	kubeUtil *kubelet.KubeUtil
	added    chan *kubelet.Pod
	removed  chan *kubelet.Pod
	stopped  chan struct{}
}

// NewFileSystemWatcher returns a new watcher.
func NewFileSystemWatcher() (*FileSystemWatcher, error) {
	// initialize a file system watcher to list added and removed pod directories.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(podsDirectoryPath); err != nil {
		return nil, err
	}
	// initialize kubeUtil to request pods information from podUIDs
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	return &FileSystemWatcher{
		watcher:  watcher,
		kubeUtil: kubeUtil,
		added:    make(chan *kubelet.Pod),
		removed:  make(chan *kubelet.Pod),
		stopped:  make(chan struct{}),
	}, nil
}

// Start starts the watcher.
func (fs *FileSystemWatcher) Start() {
	go w.run()
}

// Stop stops the watcher.
func (w *FileSystemWatcher) Stop() {
	w.watcher.Close()
	w.stopped <- struct{}{}
}

// Added returns a channel of new pods.
func (w *FileSystemWatcher) Added() chan *kubelet.Pod {
	return w.added
}

// Removed returns a channel of pods removed.
func (w *FileSystemWatcher) Removed() chan *kubelet.Pod {
	return w.removed
}

// run listens to file system events and errors.
func (w *FileSystemWatcher) run() {
	for {
		select {
		case event := <-w.watcher.Events:
			w.handle(event)
		case err := <-w.watcher.Errors:
			log.Errorf("an error occured scanning %v: %v", podsDirectoryPath, err)
		case <-w.stopped:
			return
		}
	}
}

// handle handles new events on the file system in the '/var/log/pods' directory
// to collect information about the new pods added and removed.
func (w *FileSystemWatcher) handle(event fsnotify.Event) {
	pod, err := w.getPod(event.Name)
	if err != nil {
		log.Error(err)
		return
	}
	switch event.Op {
	case fsnotify.Create:
		w.added <- pod
	case fsnotify.Remove:
		w.removed <- pod
	}
}

// getPod returns the pod reversed from its log path with format: '/var/log/pods/podUID'.
func (w *FileSystemWatcher) getPod(path string) (*kubelet.Pod, error) {
	podUID := filepath.Base(path)
	pod, err := w.kubeUtil.GetPodFromUID(podUID)
	return pod, fmt.Errorf("can't find pod with id %v: %v", podUID, err)
}
