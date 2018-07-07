// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Watcher looks for new and removed pods.
type Watcher interface {
	Start()
	Stop()
}

// Strategy represents the strategy to collect new and removed pods.
type Strategy uint32

const (
	// KubeletPolling pod discovery
	KubeletPolling Strategy = 1 << iota
	// Inotify pod discovery
	Inotify
)

// PodProvider provides the new pods and the ones that have been removed.
type PodProvider struct {
	Added   chan *kubelet.Pod
	Removed chan *kubelet.Pod
	watcher Watcher
}

// NewPodProvider returns a new pod provider.
func NewPodProvider(strategy Strategy) (*PodProvider, error) {
	added := make(chan *kubelet.Pod)
	removed := make(chan *kubelet.Pod)
	var watcher Watcher
	var err error
	switch strategy {
	case KubeletPolling:
		watcher, err = NewKubeletWatcher(added, removed)
	case Inotify:
		watcher, err = NewFileSystemWatcher(added, removed)
	default:
		return nil, fmt.Errorf("invalid watching strategy: %v", strategy)
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
