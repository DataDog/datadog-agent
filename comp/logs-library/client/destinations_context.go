// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

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

// NewDestinationsContext returns an initialized DestinationsContext
func NewDestinationsContext() *DestinationsContext {
	return &DestinationsContext{}
}

// Start creates a context that will be cancelled on Stop()
func (dc *DestinationsContext) Start() {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	dc.context, dc.cancel = context.WithCancel(context.Background())
}

// Stop cancels the context that should be used by all senders.
func (dc *DestinationsContext) Stop() {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	if dc.cancel != nil {
		dc.cancel()
		dc.cancel = nil
	}
	// Here we keep the cancelled context to make sure in-flight destination get it.
}

// Context allows one to access the current context of this DestinationsContext.
func (dc *DestinationsContext) Context() context.Context {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	return dc.context
}
