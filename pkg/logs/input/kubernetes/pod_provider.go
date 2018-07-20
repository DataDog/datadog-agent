// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Watcher looks for new and deleted pods.
type Watcher interface {
	Start()
	Stop()
}

// PodProvider provides the new pods and the ones that have been removed.
type PodProvider struct {
	Added   chan *kubelet.Pod
	Removed chan *kubelet.Pod
	watcher Watcher
}

// NewPodProvider returns a new pod provider.
func NewPodProvider(useInotify bool) (*PodProvider, error) {
	added := make(chan *kubelet.Pod)
	removed := make(chan *kubelet.Pod)
	var watcher Watcher
	var err error
	if useInotify {
		log.Info("Using inotify to watch pods")
		watcher, err = NewFileSystem(podsDirectoryPath, added, removed)
	} else {
		log.Info("Using kubelet to watch pods")
		watcher, err = NewKubelet(added, removed)
	}
	return &PodProvider{
		Added:   added,
		Removed: removed,
		watcher: watcher,
	}, err
}

// Start starts the watcher
func (p *PodProvider) Start() {
	p.watcher.Start()
}

// Stop stops the watcher
func (p *PodProvider) Stop() {
	p.watcher.Stop()
}
