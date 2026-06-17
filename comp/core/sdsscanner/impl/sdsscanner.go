// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sdsscannerimpl implements the sdsscanner component.
package sdsscannerimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	sdsscanner "github.com/DataDog/datadog-agent/comp/core/sdsscanner/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	sds "github.com/DataDog/datadog-agent/pkg/util/sds"
)

// Requires defines the dependencies for the sdsscanner component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
}

// Provides defines the output of the sdsscanner component.
type Provides struct {
	Comp sdsscanner.Component
}

// NewComponent creates a new sdsscanner component. When `sds_scanner.enabled`
// is false (the default) every scanner it hands out is a no-op, regardless of
// the build tag.
func NewComponent(reqs Requires) Provides {
	r := &registry{
		enabled:  reqs.Config.GetBool("sds_scanner.enabled"),
		scanners: make(map[string]sds.Scanner),
	}

	// Release the native scanner handles on shutdown.
	reqs.Lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error { return r.closeAll() },
	})

	return Provides{Comp: r}
}

type registry struct {
	enabled  bool
	mu       sync.Mutex
	scanners map[string]sds.Scanner
}

func (r *registry) Register(name string, rules []sds.RuleDefinition) (sds.Scanner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.register(name, rules)
}

func (r *registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unregister(name)
}

// register builds and registers a scanner under name, replacing any existing
// one. The caller must hold r.mu.
func (r *registry) register(name string, rules []sds.RuleDefinition) (sds.Scanner, error) {
	var (
		s   sds.Scanner
		err error
	)
	if r.enabled {
		s, err = sds.NewScanner(rules)
	} else {
		s = sds.NoOpScanner()
	}
	if err != nil {
		return nil, fmt.Errorf("sdsscanner: creating scanner %q: %w", name, err)
	}

	if err := r.unregister(name); err != nil {
		return nil, err
	}

	r.scanners[name] = s
	return s, nil
}

// unregister removes and closes the scanner registered under name, if any. The
// caller must hold r.mu.
func (r *registry) unregister(name string) error {
	s, ok := r.scanners[name]
	if !ok {
		return nil
	}
	delete(r.scanners, name)
	if err := s.Close(); err != nil {
		return fmt.Errorf("sdsscanner: closing scanner %q: %w", name, err)
	}
	return nil
}

func (r *registry) Get(name string) (sds.Scanner, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scanners[name]
	return s, ok
}

func (r *registry) closeAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, s := range r.scanners {
		if err := s.Close(); err != nil {
			errs = append(errs, fmt.Errorf("sdsscanner: closing scanner %q: %w", name, err))
		}
		delete(r.scanners, name)
	}
	return errors.Join(errs...)
}
