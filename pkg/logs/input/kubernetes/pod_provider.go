// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
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
func NewPodProvider() (*PodProvider, error) {
	added := make(chan *kubelet.Pod)
	removed := make(chan *kubelet.Pod)
	watcher, err := NewKubelet(added, removed)
	if err != nil {
		return nil, err
	}
	return &PodProvider{
		Added:   added,
		Removed: removed,
		watcher: watcher,
	}, nil
}

// Start starts the watcher
func (p *PodProvider) Start() {
	p.watcher.Start()
}

// Stop stops the watcher
func (p *PodProvider) Stop() {
	p.watcher.Stop()
}
