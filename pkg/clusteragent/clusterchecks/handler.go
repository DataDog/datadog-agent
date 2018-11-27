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
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	schedulerName = "clusterchecks"
)

type state int

const (
	unknown state = iota
	leader
	follower
)

// leaderIPCallback describes the leader-election method we
// need and allows to inject a custom one for tests
type leaderIPCallback func() (string, error)

// pluggableAutoConfig describes the AC methods we use and allows
// to mock it for tests (see mockedPluggableAutoConfig)
type pluggableAutoConfig interface {
	AddScheduler(string, scheduler.Scheduler, bool)
	RemoveScheduler(string)
}

// The handler is the glue holding all components for cluster-checks management
type Handler struct {
	m                    sync.RWMutex
	autoconfig           pluggableAutoConfig
	dispatcher           *dispatcher
	dispatcherCancel     context.CancelFunc
	state                state
	leaderIP             string
	warmupDuration       time.Duration
	leaderStatusFreq     time.Duration
	leaderStatusCallback leaderIPCallback
	leadershipChan       chan state
}

// NewHandler returns a populated Handler
// It will hook on the specified AutoConfig instance at Start
func NewHandler(ac pluggableAutoConfig) (*Handler, error) {
	if ac == nil {
		return nil, errors.New("empty autoconfig object")
	}
	h := &Handler{
		autoconfig:       ac,
		leaderStatusFreq: 5 * time.Second,
		warmupDuration:   config.Datadog.GetDuration("cluster_checks.warmup_duration") * time.Second,
		leadershipChan:   make(chan state, 1),
		dispatcher:       newDispatcher(),
	}

	if config.Datadog.GetBool("leader_election") {
		callback, err := getLeaderIPCallback()
		if err != nil {
			return nil, err
		}
		h.leaderStatusCallback = callback
	}

	return h, nil
}

// Run is the main goroutine for the handler. It has to
// be called in a goroutine with a cancellable context.
func (h *Handler) Run(ctx context.Context) {
	h.m.Lock()
	if h.leaderStatusCallback != nil {
		go h.leaderWatch(ctx)
	} else {
		// With no leader election enabled, we assume only one DCA is running
		h.state = leader
		h.leadershipChan <- leader
	}
	h.m.Unlock()

	for {
		// Follower / unknown
		select {
		case <-ctx.Done():
			return
		case newState := <-h.leadershipChan:
			if newState != leader {
				// Still follower, go back to select
				continue
			}
		}

		// Leading, start warmup
		log.Infof("Becoming leader, waiting %s for node-agents to report", h.warmupDuration)
		select {
		case <-ctx.Done():
			return
		case newState := <-h.leadershipChan:
			if newState != leader {
				continue
			}
		case <-time.After(h.warmupDuration):
			break
		}

		// Run discovery and dispatching
		log.Info("Warmup phase finished, starting to serve configurations")
		dispatchCtx, dispatchCancel := context.WithCancel(ctx)
		go h.runDispatch(dispatchCtx)

		// Wait until we lose leadership or exit
		for {
			var newState state

			select {
			case <-ctx.Done():
				dispatchCancel()
				return
			case newState = <-h.leadershipChan:
				// Store leadership status
			}

			if newState != leader {
				log.Info("Lost leadership, reverting to follower")
				dispatchCancel()
				break // Return back to main loop start
			}
		}
	}
}

// startDispatch hooks to Autodiscovery and manages checks
func (h *Handler) runDispatch(ctx context.Context) {
	// Register our scheduler and ask for a config replay
	h.autoconfig.AddScheduler(schedulerName, h.dispatcher, true)

	// Run dispatcher cleanup loop - blocking until context is cancelled
	h.dispatcher.run(ctx)

	h.m.Lock()
	defer h.m.Unlock()
	// Churn the dispatcher and restart fresh
	h.dispatcher = newDispatcher()
	h.autoconfig.RemoveScheduler(schedulerName)
}

func (h *Handler) leaderWatch(ctx context.Context) {
	err := h.updateLeaderIP(true)
	if err != nil {
		log.Warnf("Could not refresh leadership status: %s", err)
	}

	watchTicker := time.NewTicker(h.leaderStatusFreq)
	defer watchTicker.Stop()

	for {
		select {
		case <-watchTicker.C:
			err := h.updateLeaderIP(false)
			if err != nil {
				log.Warnf("Could not refresh leadership status: %s", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// updateLeaderIP queries the leader election engine and updates
// the leader IP accordlingly. In case of leadership statuschange,
// a boolean is sent on leadershipChan:
//   - true: becoming leader
//   - false: lost leadership
func (h *Handler) updateLeaderIP(firstRun bool) error {
	h.m.Lock()
	defer h.m.Unlock()

	newIP, err := h.leaderStatusCallback()
	if err != nil {
		return err
	}

	// Fast exit if no change
	if !firstRun && (newIP == h.leaderIP) {
		return nil
	}

	// Test if the leader/follower status changed
	newState := follower
	if newIP == "" {
		newState = leader
	}
	statusChange := h.state != newState

	// Store new state
	h.leaderIP = newIP
	h.state = newState

	// Notify leadership status change
	if firstRun || statusChange {
		h.leadershipChan <- newState
	}
	return nil
}
