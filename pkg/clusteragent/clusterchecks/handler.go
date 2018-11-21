// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
)

const (
	schedulerName = "clusterchecks"
)

// The handler is the glue holding all components for cluster-checks management
type Handler struct {
	m          sync.Mutex
	autoconfig *autodiscovery.AutoConfig
	dispatcher *dispatcher
}

// NewHandler returns a populated Handler
// It will hook on the specified AutoConfig instance at Start
func NewHandler(ac *autodiscovery.AutoConfig) (*Handler, error) {
	if ac == nil {
		return nil, errors.New("empty autoconfig object")
	}
	h := &Handler{
		autoconfig: ac,
	}
	return h, nil
}

// Run is the main goroutine for the handler. It has to
// be called in a goroutine with a cancellable context.
func (h *Handler) Run(ctx context.Context) {
	h.startDiscovery(ctx)
	<-ctx.Done()
	h.stopDiscovery()
}

// startDiscovery hooks to Autodiscovery and starts managing checks
func (h *Handler) startDiscovery(ctx context.Context) {
	h.m.Lock()
	defer h.m.Unlock()
	// Clean initial state
	h.dispatcher = newDispatcher()
	go h.dispatcher.cleanupLoop(ctx)

	// Register our scheduler and ask for a config replay
	h.autoconfig.AddScheduler(schedulerName, h.dispatcher, true)
}

// stopDiscovery stops the management logic and un-hooks from Autodiscovery
func (h *Handler) stopDiscovery() {
	h.m.Lock()
	defer h.m.Unlock()

	// AD will call dispatcher.Stop for us
	h.autoconfig.RemoveScheduler(schedulerName)

	// Release memory
	h.dispatcher = nil
}
