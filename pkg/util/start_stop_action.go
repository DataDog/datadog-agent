// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"sync"
)

// StartStopAction is an helper to implement Start / Stop pattern
// in a thread safe way.
type StartStopAction struct {
	start       bool
	startMut    sync.Mutex
	cancel      func()
	stopContext context.Context
	stopped     chan bool
}

// NewStartStopAction creates a new instance of StartStopAction
func NewStartStopAction() *StartStopAction {
	context, cancel := context.WithCancel(context.Background())
	return &StartStopAction{
		start:       false,
		cancel:      cancel,
		stopContext: context,
		stopped:     make(chan bool),
	}
}

// Start runs `start` function. If `Start` was already called, the call does nothing.
// This function is thread safe.
func (s *StartStopAction) Start(start func(context.Context)) {
	s.startMut.Lock()
	if s.start {
		s.startMut.Unlock()
		return
	}
	s.start = true
	s.startMut.Unlock()
	go func() {
		start(s.stopContext)
		close(s.stopped)
	}()
}

// Stop sends a stop signal to the function run by `Start` and waits until its completion.
// If `Start` was not called or `Stop` was already called, this function does nothing.
// `Start` cannot be called after calling `Stop`.
// This function is thread safe.
func (s *StartStopAction) Stop() {
	s.startMut.Lock()
	defer s.startMut.Unlock()
	s.cancel()
	if s.start {
		<-s.stopped
		s.start = false
	}
}
