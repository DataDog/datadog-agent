// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
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
	store      *clusterStore
	running    bool
}

// SetupHandler returns a populated Handler
// If will hook on the specified AutoConfig instance at Start
func SetupHandler(ac *autodiscovery.AutoConfig) (*Handler, error) {
	h := &Handler{
		autoconfig: ac,
	}
	return h, nil
}

// StartDiscovery hooks to Autodiscovery and starts managing checks
func (h *Handler) StartDiscovery() error {
	h.m.Lock()
	defer h.m.Unlock()
	if h.running {
		return errors.New("already running")
	}

	// Clean initial state
	h.store = newClusterStore()
	h.dispatcher = newDispatcher(h.store)
	h.running = true

	// Register our scheduler and ask for a config replay
	h.autoconfig.AddScheduler(schedulerName, h.dispatcher, true)

	return nil
}

// StopDiscovery stops the management logic and un-hooks from Autodiscovery
func (h *Handler) StopDiscovery() error {
	h.m.Lock()
	defer h.m.Unlock()
	if !h.running {
		return errors.New("not running")
	}

	// AD will call dispatcher.Stop for us
	h.autoconfig.RemoveScheduler(schedulerName)

	// Release memory
	h.dispatcher = nil
	h.store = nil

	return nil
}
