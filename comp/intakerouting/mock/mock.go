// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock of the intake routing component.
package mock

import (
	"context"
	"slices"
	"sync"

	"go.uber.org/fx"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock is a controllable intake routing component. Endpoints report their default until a route
// is set for them with SetRoute.
type Mock interface {
	intakerouting.Component

	// SetRoute makes every endpoint of type typ whose default host is defaultHost report ep, or its
	// default again when ep is nil. Open registrations are notified.
	SetRoute(typ intakerouting.TelemetryType, defaultHost string, ep *intakerouting.Endpoint)
	// Requests returns every request registered so far, in order.
	Requests() []intakerouting.Request
}

type routeKey struct {
	typ  intakerouting.TelemetryType
	host string
}

type mock struct {
	mu       sync.Mutex
	routes   map[routeKey]intakerouting.Endpoint
	requests []intakerouting.Request
	regs     map[*registration]struct{}
}

// New returns a new mock.
func New() Mock {
	return &mock{
		routes: make(map[routeKey]intakerouting.Endpoint),
		regs:   make(map[*registration]struct{}),
	}
}

// Module provides the mock as both Mock and intakerouting.Component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(New),
		fx.Provide(func(m Mock) intakerouting.Component { return m }),
	)
}

func (m *mock) Register(reqs []intakerouting.Request) intakerouting.Registration {
	m.mu.Lock()
	defer m.mu.Unlock()
	reg := &registration{mock: m, reqs: slices.Clone(reqs), changes: make(chan struct{}, 1)}
	m.regs[reg] = struct{}{}
	m.requests = append(m.requests, reqs...)
	return reg
}

func (m *mock) SetRoute(typ intakerouting.TelemetryType, defaultHost string, ep *intakerouting.Endpoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := routeKey{typ: typ, host: defaultHost}
	if ep == nil {
		delete(m.routes, key)
	} else {
		m.routes[key] = *ep
	}
	for reg := range m.regs {
		select {
		case reg.changes <- struct{}{}:
		default:
		}
	}
}

func (m *mock) Requests() []intakerouting.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.requests)
}

type registration struct {
	mock    *mock
	reqs    []intakerouting.Request
	changes chan struct{}
}

func (r *registration) WaitInitial(context.Context) []intakerouting.Endpoint {
	return r.Current()
}

func (r *registration) Current() []intakerouting.Endpoint {
	r.mock.mu.Lock()
	defer r.mock.mu.Unlock()
	endpoints := make([]intakerouting.Endpoint, len(r.reqs))
	for i, req := range r.reqs {
		if ep, ok := r.mock.routes[routeKey{typ: req.Type, host: req.Default.Host}]; ok {
			endpoints[i] = ep
		} else {
			endpoints[i] = req.Default
		}
	}
	return endpoints
}

func (r *registration) Changes() <-chan struct{} {
	return r.changes
}

func (r *registration) Close() {
	r.mock.mu.Lock()
	defer r.mock.mu.Unlock()
	delete(r.mock.regs, r)
}
