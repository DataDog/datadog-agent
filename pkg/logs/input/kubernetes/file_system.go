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

// FileSystem looks for new and deleted pods listening to file system events.
type FileSystem struct {
	watcher          *fsnotify.Watcher
	kubeUtil         *kubelet.KubeUtil
	podsDirectory    string
	podsPerDirectory map[string]*kubelet.Pod
	added            chan *kubelet.Pod
	removed          chan *kubelet.Pod
	stopped          chan struct{}
}

// NewFileSystem returns a new watcher.
func NewFileSystem(podsDirectory string, added, removed chan *kubelet.Pod) (*FileSystem, error) {
	// initialize a file system watcher to list added and deleted pod directories.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(podsDirectory); err != nil {
		return nil, err
	}
	// initialize kubeUtil to request pods information from podUIDs
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	return &FileSystem{
		watcher:          watcher,
		kubeUtil:         kubeUtil,
		podsDirectory:    podsDirectory,
		podsPerDirectory: make(map[string]*kubelet.Pod),
		added:            added,
		removed:          removed,
		stopped:          make(chan struct{}),
	}, nil
}

// Start starts the watcher.
func (w *FileSystem) Start() {
	w.addExistingPods()
	go w.run()
}

// Stop stops the watcher.
func (w *FileSystem) Stop() {
	w.watcher.Close()
	w.stopped <- struct{}{}
}

// addExistingPods retrieves all pods in /var/log/pods at start and pass them along to the channel.
func (w *FileSystem) addExistingPods() {
	directories, err := filepath.Glob(fmt.Sprintf("%s/*", w.podsDirectory))
	if err != nil {
		log.Warnf("Can't retrieve pod directories: %v", err)
	}
	for _, directory := range directories {
		pod, err := w.getPod(directory)
		if err != nil {
			log.Warn(err)
			continue
		}
		w.podsPerDirectory[directory] = pod
		w.added <- pod
	}
}

// run listens to file system events and errors.
func (w *FileSystem) run() {
	for {
		select {
		case event := <-w.watcher.Events:
			w.handle(event)
		case err := <-w.watcher.Errors:
			log.Warnf("An error occurred scanning %v: %v", w.podsDirectory, err)
		case <-w.stopped:
			return
		}
	}
}

// handle handles new events on the file system in the '/var/log/pods' directory
// to collect the new pods added and removed.
func (w *FileSystem) handle(event fsnotify.Event) {
	directory := event.Name
	switch event.Op {
	case fsnotify.Create:
		pod, err := w.getPod(directory)
		if err != nil {
			log.Warn(err)
			return
		}
		w.podsPerDirectory[directory] = pod
		w.added <- pod
	case fsnotify.Remove:
		if pod, exists := w.podsPerDirectory[directory]; exists {
			delete(w.podsPerDirectory, directory)
			w.removed <- pod
		}
	}
}

// getPod returns the pod reversed from its log path with format '/var/log/pods/podUID'.
func (w *FileSystem) getPod(path string) (*kubelet.Pod, error) {
	podUID := filepath.Base(path)
	pod, err := w.kubeUtil.GetPodFromUID(podUID)
	if err != nil {
		return nil, fmt.Errorf("can't find pod with id %v: %v", podUID, err)
	}
	return pod, nil
}
