// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package fgmimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	fgmdef "github.com/DataDog/datadog-agent/comp/fgm/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// noopComponent is a no-op implementation for when FGM is disabled
type noopComponent struct{}

// Requires defines the dependencies for the FGM component
type Requires struct {
	fx.In

	Lc           fx.Lifecycle
	Log          log.Component
	Config       config.Component
	Workloadmeta workloadmeta.Component
	Observer     option.Option[observer.Component]
}

type fgmComponent struct {
	log            log.Component
	config         config.Component
	store          workloadmeta.Component
	observerHandle observer.Handle

	mu               sync.RWMutex
	activeContainers map[string]*trackedContainer // containerID → metadata

	cancel context.CancelFunc
}

type trackedContainer struct {
	id         string
	cgroupPath string
	pid        int
	labels     map[string]string
}

// NewComponent creates a new FGM component instance
func NewComponent(reqs Requires) (fgmdef.Component, error) {
	// Check if Observer is available
	obs, ok := reqs.Observer.Get()
	if !ok {
		reqs.Log.Info("fgm: Observer component not available, disabling FGM input source")
		return &noopComponent{}, nil
	}

	// Check if enabled in config
	if !reqs.Config.GetBool("fgm.enabled") {
		reqs.Log.Info("fgm: Disabled via configuration")
		return &noopComponent{}, nil
	}

	c := &fgmComponent{
		log:              reqs.Log,
		config:           reqs.Config,
		store:            reqs.Workloadmeta,
		observerHandle:   obs.GetHandle("fgm"),
		activeContainers: make(map[string]*trackedContainer),
	}

	reqs.Lc.Append(fx.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return c, nil
}

func (c *fgmComponent) start(ctx context.Context) error {
	// Initialize Rust FFI
	if err := fgmInit(); err != nil {
		return fmt.Errorf("fgm: failed to initialize Rust library: %w", err)
	}

	c.log.Info("fgm: Started FGM input source")

	ctx, c.cancel = context.WithCancel(ctx)

	// Start event processing goroutine
	go c.runEventLoop(ctx)

	// Start sampling goroutine
	go c.runSamplingLoop(ctx)

	return nil
}

func (c *fgmComponent) stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}

	fgmShutdown()
	c.log.Info("fgm: Stopped FGM input source")
	return nil
}

func (c *fgmComponent) runEventLoop(ctx context.Context) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		Build()

	eventCh := c.store.Subscribe("fgm", workloadmeta.NormalPriority, filter)
	defer c.store.Unsubscribe(eventCh)

	for {
		select {
		case bundle, ok := <-eventCh:
			if !ok {
				return
			}
			c.processEvents(bundle.Events)
			bundle.Acknowledge()

		case <-ctx.Done():
			return
		}
	}
}

func (c *fgmComponent) processEvents(events []workloadmeta.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			// Only track running containers with cgroup paths
			if container.State.Running && container.CgroupPath != "" {
				c.activeContainers[container.ID] = &trackedContainer{
					id:         container.ID,
					cgroupPath: c.normalizeCgroupPath(container.CgroupPath),
					pid:        container.PID,
					labels:     container.Labels,
				}
				c.log.Debugf("fgm: Now tracking container %s (cgroup: %s, pid: %d)",
					container.ID[:min(12, len(container.ID))], container.CgroupPath, container.PID)
			} else if !container.State.Running {
				// Container stopped
				delete(c.activeContainers, container.ID)
				c.log.Debugf("fgm: Container %s stopped, removed from tracking",
					container.ID[:min(12, len(container.ID))])
			}

		case workloadmeta.EventTypeUnset:
			delete(c.activeContainers, container.ID)
			c.log.Debugf("fgm: Container %s removed, stopped tracking",
				container.ID[:min(12, len(container.ID))])
		}
	}
}

func (c *fgmComponent) normalizeCgroupPath(path string) string {
	// WorkloadMeta may provide relative paths
	// Ensure we have absolute path to /sys/fs/cgroup
	if len(path) > 0 && path[0] != '/' {
		return "/sys/fs/cgroup/" + path
	}
	return path
}

func (c *fgmComponent) runSamplingLoop(ctx context.Context) {
	interval := c.config.GetDuration("fgm.sample_interval")
	if interval == 0 {
		interval = 1 * time.Second // Default 1Hz
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sampleAllContainers()

		case <-ctx.Done():
			return
		}
	}
}

func (c *fgmComponent) sampleAllContainers() {
	// Snapshot active containers (avoid holding lock during sampling)
	c.mu.RLock()
	containers := make([]*trackedContainer, 0, len(c.activeContainers))
	for _, tc := range c.activeContainers {
		containers = append(containers, tc)
	}
	c.mu.RUnlock()

	// Sample each container
	for _, tc := range containers {
		if err := c.sampleContainer(tc); err != nil {
			c.log.Warnf("fgm: Failed to sample container %s: %v",
				tc.id[:min(12, len(tc.id))], err)
		}
	}
}

// min returns the minimum of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
