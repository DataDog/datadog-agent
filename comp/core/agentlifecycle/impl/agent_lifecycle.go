// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentlifecycleimpl implements the experimental prepared/active Agent lifecycle.
package agentlifecycleimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"

	agentlifecycle "github.com/DataDog/datadog-agent/comp/core/agentlifecycle/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

const (
	lockRetryInterval   = 100 * time.Millisecond
	rolloutEnabledKey   = "experimental.node_agent_rollout.enabled"
	rolloutLockPathKey  = "experimental.node_agent_rollout.lock_path"
	rolloutStatePathKey = "experimental.node_agent_rollout.state_path"
)

type fileLocker interface {
	TryLockContext(context.Context, time.Duration) (bool, error)
	Unlock() error
}

type dependencies struct {
	compdef.In

	Config config.Component
	Log    log.Component
	Params agentlifecycle.Params
}

type component struct {
	enabled       bool
	componentName string
	lockPath      string
	statePath     string
	log           log.Component
	locker        fileLocker

	mu       sync.Mutex
	acquired bool
	closed   bool
}

var _ agentlifecycle.Component = (*component)(nil)

// NewComponent creates the experimental Agent lifecycle component.
func NewComponent(deps dependencies) (agentlifecycle.Component, error) {
	return newComponent(deps, func(path string) fileLocker { return flock.New(path) })
}

func newComponent(deps dependencies, newLocker func(string) fileLocker) (agentlifecycle.Component, error) {
	if !deps.Config.GetBool(rolloutEnabledKey) {
		return &component{}, nil
	}

	lockPath := deps.Config.GetString(rolloutLockPathKey)
	statePath := deps.Config.GetString(rolloutStatePathKey)
	if deps.Params.ComponentName == "" {
		return nil, errors.New("experimental node Agent rollout requires a component name")
	}
	if !filepath.IsAbs(lockPath) {
		return nil, fmt.Errorf("%s must be an absolute path", rolloutLockPathKey)
	}
	if !filepath.IsAbs(statePath) {
		return nil, fmt.Errorf("%s must be an absolute path", rolloutStatePathKey)
	}
	if filepath.Clean(lockPath) == filepath.Clean(statePath) {
		return nil, errors.New("experimental node Agent rollout lock and state paths must differ")
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create Agent rollout lock directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return nil, fmt.Errorf("create Agent rollout state directory: %w", err)
	}

	return &component{
		enabled:       true,
		componentName: deps.Params.ComponentName,
		lockPath:      lockPath,
		statePath:     statePath,
		log:           deps.Log,
		locker:        newLocker(lockPath),
	}, nil
}

func (c *component) Wait(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("experimental Agent lifecycle is already closed")
	}
	if c.acquired {
		c.mu.Unlock()
		return errors.New("experimental Agent lifecycle already owns the node lock")
	}
	if err := c.writeState(agentlifecycle.StatePrepared); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	c.log.Infof("%s is prepared and waiting for node ownership at %s", c.componentName, c.lockPath)
	locked, err := c.locker.TryLockContext(ctx, lockRetryInterval)
	if err != nil {
		return fmt.Errorf("acquire Agent rollout lock %q: %w", c.lockPath, err)
	}
	if !locked {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("failed to acquire Agent rollout lock %q", c.lockPath)
	}

	c.mu.Lock()
	c.acquired = true
	if err := c.writeState(agentlifecycle.StateActivating); err != nil {
		c.acquired = false
		c.mu.Unlock()
		unlockErr := c.locker.Unlock()
		return errors.Join(err, unlockErr)
	}
	c.mu.Unlock()
	c.log.Infof("%s acquired node ownership and is activating", c.componentName)
	return nil
}

func (c *component) MarkActive() error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("cannot mark a closed experimental Agent lifecycle active")
	}
	if !c.acquired {
		return errors.New("cannot mark the experimental Agent lifecycle active before acquiring node ownership")
	}
	if err := c.writeState(agentlifecycle.StateActive); err != nil {
		return err
	}
	c.log.Infof("%s is active", c.componentName)
	return nil
}

func (c *component) Close() error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	stateErr := c.writeState(agentlifecycle.StateStopped)
	var unlockErr error
	if c.acquired {
		unlockErr = c.locker.Unlock()
		c.acquired = false
	}
	return errors.Join(stateErr, unlockErr)
}

func (c *component) writeState(state string) error {
	tmp, err := os.CreateTemp(filepath.Dir(c.statePath), ".agent-rollout-state-")
	if err != nil {
		return fmt.Errorf("create temporary Agent rollout state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := fmt.Fprintln(tmp, state); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write Agent rollout state: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set Agent rollout state permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Agent rollout state: %w", err)
	}
	if err := os.Rename(tmpPath, c.statePath); err != nil {
		return fmt.Errorf("publish Agent rollout state: %w", err)
	}
	return nil
}
