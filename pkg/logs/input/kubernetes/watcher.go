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
	// KubeletPolling pod discovery
	KubeletPolling Strategy = 1 << iota
	// Inotify pod discovery
	Inotify
)

// NewWatcher returns a new watcher.
func NewWatcher(strategy Strategy) (Watcher, error) {
	switch strategy {
	case KubeletPolling:
		return NewKubeletWatcher()
	case Inotify:
		return NewFileSystemWatcher()
	}
	return nil, fmt.Errorf("invalid watching strategy: %v", strategy)
}
