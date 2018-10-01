// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"context"
	"sync"
)

// A DestinationsContext manages senders and allows us to "unclog" the pipeline
// when trying to stop it and failing to send messages.
type DestinationsContext struct {
	context context.Context
	cancel  context.CancelFunc
	mutex   sync.Mutex
}

// NewDestinationsContext returns an initialized ConnectionManager
func NewDestinationsContext() *DestinationsContext {
	return &DestinationsContext{}
}

// Start creates a context that will be cancelled on Stop()
func (sm *DestinationsContext) Start() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.context, sm.cancel = context.WithCancel(context.Background())
}

// Stop cancels the context that should be used by all senders.
func (sm *DestinationsContext) Stop() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	if sm.cancel != nil {
		sm.cancel()
		sm.cancel = nil
	}
	// Here we keep the cancelled context to make sure in-flight destination get it.
}

// Context allows one to access the current context of this DestinationsContext.
func (sm *DestinationsContext) Context() context.Context {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	return sm.context
}
