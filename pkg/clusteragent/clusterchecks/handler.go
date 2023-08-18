// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	schedulerName = "clusterchecks"
	logFrequency  = 100
)

type state int

const (
	unknown state = iota
	leader
	follower
)

// pluggableAutoConfig describes the AC methods we use and allows
// to mock it for tests (see mockedPluggableAutoConfig)
type pluggableAutoConfig interface {
	AddScheduler(string, scheduler.Scheduler, bool)
	RemoveScheduler(string)
}

// Handler is the glue holding all components for cluster-checks management
type Handler struct {
	autoconfig           pluggableAutoConfig
	dispatcher           *dispatcher
	leaderStatusFreq     time.Duration
	warmupDuration       time.Duration
	leaderStatusCallback types.LeaderIPCallback
	leadershipChan       chan state
	leaderForwarder      *api.LeaderForwarder
	m                    sync.RWMutex // Below fields protected by the mutex
	state                state
	leaderIP             string
	port                 int
	errCount             int
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
		port:             config.Datadog.GetInt("cluster_agent.cmd_port"),
	}

	if config.Datadog.GetBool("leader_election") {
		h.leaderForwarder = api.NewLeaderForwarder(h.port, config.Datadog.GetInt("cluster_agent.max_leader_connections"))
		callback, err := getLeaderIPCallback()
		if err != nil {
			return nil, err
		}
		h.leaderStatusCallback = callback
	}

	// Cache a pointer to the handler for the agent status command
	key := cache.BuildAgentKey(handlerCacheKey)
	cache.Cache.Set(key, h, cache.NoExpiration)

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

// runDispatch hooks in the Autodiscovery and runs the dispatch's run method
func (h *Handler) runDispatch(ctx context.Context) {
	// Register our scheduler and ask for a config replay
	h.autoconfig.AddScheduler(schedulerName, h.dispatcher, true)

	// Run dispatcher loop - blocking until context is cancelled
	h.dispatcher.run(ctx)

	// Reset the dispatcher
	h.dispatcher.reset()
	h.autoconfig.RemoveScheduler(schedulerName)
}

func (h *Handler) leaderWatch(ctx context.Context) {
	err := h.updateLeaderIP()
	if err != nil {
		log.Warnf("Could not refresh leadership status: %s", err)
	}

	healthProbe := health.RegisterLiveness("clusterchecks-leadership")
	defer health.Deregister(healthProbe) //nolint:errcheck

	watchTicker := time.NewTicker(h.leaderStatusFreq)
	defer watchTicker.Stop()

	for {
		select {
		case <-healthProbe.C:
			// This goroutine might hang if the leader election engine blocks
		case <-watchTicker.C:
			err := h.updateLeaderIP()
			h.m.Lock()
			if err != nil {
				h.errCount++
				if h.errCount == 1 {
					log.Warnf("Could not refresh leadership status: %s, will only log every %d errors", err, logFrequency)
				} else if h.errCount%logFrequency == 0 {
					log.Warnf("Could not refresh leadership status after %d tries: %s", logFrequency, err)
				}
			} else {
				if h.errCount > 0 {
					log.Infof("Found leadership status after %d tries", h.errCount)
					h.errCount = 0
				}
			}
			h.m.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// updateLeaderIP queries the leader election engine and updates
// the leader IP accordlingly. In case of leadership statuschange,
// a state type is sent on leadershipChan.
func (h *Handler) updateLeaderIP() error {
	newIP, err := h.leaderStatusCallback()
	if err != nil {
		return err
	}

	// Lock after the leader engine call returns
	h.m.Lock()
	defer h.m.Unlock()

	var newState state
	if h.leaderForwarder != nil && newIP != h.leaderIP {
		// Update LeaderForwarder with new IP
		h.leaderForwarder.SetLeaderIP(newIP)
	}

	h.leaderIP = newIP

	switch h.state {
	case leader:
		if newIP != "" {
			newState = follower
		}
	case follower:
		if newIP == "" {
			newState = leader
		}
	case unknown:
		if newIP == "" {
			newState = leader
		} else {
			newState = follower
		}
	}

	if newState != unknown {
		h.state = newState
		h.leadershipChan <- newState
	}

	return nil
}
