// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package tailers provides tailers for API logs
package tailers

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// WrappedSource wraps a LogSource and adds/removes it to the logs-agent on
// start/stop.
type WrappedSource struct {
	// Source is the LogSource to add or remove
	Source *sources.LogSource

	// Sources is the container in which Source is added or removed.
	Sources *sources.LogSources

	// mu protects last.
	mu sync.Mutex
	// last is closed once the most recently requested add or remove is done.
	last chan struct{}
}

// Start implements Tailer#Start.
func (t *WrappedSource) Start() error {

	// this method is typically called while the container launcher is handling
	// a channel message from `sources.AddSource`; if we send this
	// synchronously, it causes a deadlock because the added source is
	// delivered to the container launcher.  As a workaround, add the source
	// in a temporary goroutine.  The long-term fix is that launchers should
	// not be adding sources.
	//
	// The goroutines are chained so that the add and the remove happen in the
	// order Start and Stop were called: otherwise a Stop shortly following a
	// Start could remove the source before it is added, leaving it behind.
	t.async(func() { t.Sources.AddSource(t.Source) })

	return nil
}

// Stop implements Tailer#Stop.
func (t *WrappedSource) Stop() {
	// (see comment in Start())
	t.async(func() { t.Sources.RemoveSource(t.Source) })
}

// async runs op in a new goroutine, once the previously requested op is done.
func (t *WrappedSource) async(op func()) {
	t.mu.Lock()
	prev := t.last
	done := make(chan struct{})
	t.last = done
	t.mu.Unlock()

	go func() {
		defer close(done)
		if prev != nil {
			<-prev
		}
		op()
	}()
}
