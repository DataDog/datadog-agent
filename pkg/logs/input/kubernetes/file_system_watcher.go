// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FileSystemWatcher looks for new and deleted pods listening to file system events.
type FileSystemWatcher struct {
	watcher  *fsnotify.Watcher
	kubeUtil *kubelet.KubeUtil
	added    chan *kubelet.Pod
	removed  chan *kubelet.Pod
	stopped  chan struct{}
}

// NewFileSystemWatcher returns a new watcher.
func NewFileSystemWatcher(added, removed chan *kubelet.Pod) (*FileSystemWatcher, error) {
	// initialize a file system watcher to list added and deleted pod directories.
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
		added:    added,
		removed:  removed,
		stopped:  make(chan struct{}),
	}, nil
}

// Start starts the watcher.
func (w *FileSystemWatcher) Start() {
	w.addExistingPods()
	go w.run()
}

// Stop stops the watcher.
func (w *FileSystemWatcher) Stop() {
	w.watcher.Close()
	w.stopped <- struct{}{}
}

// podDirectoriesPattern represents the pattern to match all pod directories.
var podDirectoriesPattern = fmt.Sprintf("%s/*", podsDirectoryPath)

// addExistingPods retrieves all pods in /var/log/pods at start and pass them along to the channel.
func (w *FileSystemWatcher) addExistingPods() {
	directories, err := filepath.Glob(podDirectoriesPattern)
	if err != nil {
		log.Warnf("Can't retrieve pod directories: %v", err)
	}
	for _, directory := range directories {
		pod, err := w.getPod(directory)
		if err != nil {
			log.Warn(err)
			continue
		}
		w.added <- pod
	}
}

// run listens to file system events and errors.
func (w *FileSystemWatcher) run() {
	for {
		select {
		case event := <-w.watcher.Events:
			w.handle(event)
		case err := <-w.watcher.Errors:
			log.Errorf("an error occurred scanning %v: %v", podsDirectoryPath, err)
		case <-w.stopped:
			return
		}
	}
}

// handle handles new events on the file system in the '/var/log/pods' directory
// to collect the new pods added and removed.
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
	if err != nil {
		return nil, fmt.Errorf("can't find pod with id %v: %v", podUID, err)
	}
	return pod, nil
}
