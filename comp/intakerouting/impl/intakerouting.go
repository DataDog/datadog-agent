// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package intakeroutingimpl implements the intake routing component.
package intakeroutingimpl

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const (
	enabledKey      = "intake_routing.enabled"
	startupGraceKey = "intake_routing.startup_grace_period"
	refreshKey      = "intake_routing.refresh_interval"

	maxStartupGrace    = 30 * time.Second
	minRefreshInterval = time.Minute
)

// Requires defines the dependencies of the intake routing component.
type Requires struct {
	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component
}

// Provides defines the output of the intake routing component.
type Provides struct {
	Comp intakerouting.Component
}

// NewComponent creates the intake routing component. When intake routing is disabled, it returns
// a component that always reports the default endpoints and runs nothing.
func NewComponent(reqs Requires) Provides {
	if !reqs.Config.GetBool(enabledKey) {
		return Provides{Comp: disabledComponent{}}
	}

	grace, refresh := loadSettings(reqs.Config, reqs.Log)
	// The path checker reads the proxy settings of the transport, which are those telemetry is
	// sent with.
	transport := httputils.CreateHTTPTransport(reqs.Config)
	r := newRouter(routerOptions{
		log:        reqs.Log,
		client:     newHTTPRoutingClientWithTransport(transport),
		paths:      newNetPathChecker(transport),
		clock:      clock.New(),
		tlm:        newRoutingTelemetry(reqs.Telemetry),
		grace:      grace,
		refresh:    refresh,
		keyRecheck: keyRecheckInterval,
		jitter: func(d time.Duration) time.Duration {
			return time.Duration(float64(d) * (1 + refreshJitter*(2*rand.Float64()-1)))
		},
		startupBackoffRandomization: startupBackoffRandomization,
	})
	reqs.Lc.Append(compdef.Hook{OnStop: r.stop})
	reqs.Log.Infof("Intake routing is enabled (startup grace period: %s, refresh interval: %s)", grace, refresh)
	return Provides{Comp: r}
}

// loadSettings reads the tunables and clamps them to safe values.
func loadSettings(cfg config.Component, logger log.Component) (grace, refresh time.Duration) {
	grace = cfg.GetDuration(startupGraceKey)
	if clamped := min(max(grace, 0), maxStartupGrace); clamped != grace {
		logger.Warnf("%s is %s, using %s instead (allowed range: 0s to %s)", startupGraceKey, grace, clamped, maxStartupGrace)
		grace = clamped
	}
	refresh = cfg.GetDuration(refreshKey)
	if refresh < minRefreshInterval {
		logger.Warnf("%s is %s, using %s instead (minimum: %s)", refreshKey, refresh, minRefreshInterval, minRefreshInterval)
		refresh = minRefreshInterval
	}
	return grace, refresh
}

// disabledComponent is used when intake routing is disabled.
type disabledComponent struct{}

func (disabledComponent) Register(reqs []intakerouting.Request) intakerouting.Registration {
	endpoints := make([]intakerouting.Endpoint, len(reqs))
	for i, req := range reqs {
		endpoints[i] = req.Default
	}
	return &staticRegistration{endpoints: endpoints, changes: make(chan struct{})}
}

// staticRegistration always reports the default endpoints.
type staticRegistration struct {
	endpoints []intakerouting.Endpoint
	changes   chan struct{}
}

func (s *staticRegistration) WaitInitial(context.Context) []intakerouting.Endpoint {
	return s.Current()
}

func (s *staticRegistration) Current() []intakerouting.Endpoint {
	return slices.Clone(s.endpoints)
}

func (s *staticRegistration) Changes() <-chan struct{} {
	return s.changes
}

func (s *staticRegistration) Close() {}

type routerOptions struct {
	log     log.Component
	client  routingClient
	paths   pathChecker
	clock   clock.Clock
	tlm     *routingTelemetry
	grace   time.Duration
	refresh time.Duration
	// keyRecheck is how often the API keys of the routes are read again.
	keyRecheck time.Duration
	// jitter randomizes the refresh interval.
	jitter                      func(time.Duration) time.Duration
	startupBackoffRandomization float64
}

// router is the intake routing component when it is enabled. Registered endpoints are grouped by
// site and API key: one query answers every endpoint of a group. A single goroutine (loop, in
// scheduler.go) sends the queries and applies the answers. It starts with the first registration.
type router struct {
	routerOptions

	ctx    context.Context
	cancel context.CancelFunc
	// wake asks the loop to run a pass now.
	wake chan struct{}
	// done is closed when the loop exits.
	done chan struct{}
	// afterPass, if set, is called each time the loop goes idle. Tests use it to synchronize with
	// the loop.
	afterPass func()

	// mu guards everything below, and the mutable fields of registrations, routes and groups. It is
	// never held during network I/O.
	mu          sync.Mutex
	stopped     bool
	loopStarted bool
	regs        map[*registration]struct{}
	groups      map[groupKey]*group
	// types holds every telemetry type registered so far, so active_overrides drops back to 0.
	types map[intakerouting.TelemetryType]struct{}
}

func newRouter(opts routerOptions) *router {
	ctx, cancel := context.WithCancel(context.Background())
	return &router{
		routerOptions: opts,
		ctx:           ctx,
		cancel:        cancel,
		wake:          make(chan struct{}, 1),
		done:          make(chan struct{}),
		regs:          make(map[*registration]struct{}),
		groups:        make(map[groupKey]*group),
		types:         make(map[intakerouting.TelemetryType]struct{}),
	}
}

// registration implements intakerouting.Registration.
type registration struct {
	router *router
	routes []*route
	// deadline is the end of the startup grace period.
	deadline time.Time
	changes  chan struct{}
	// initialDone is closed once every route has its first decision.
	initialDone chan struct{}
	// closedCh is closed by Close.
	closedCh chan struct{}

	// Guarded by router.mu.
	closed        bool
	initialClosed bool
}

// route is the routing state of one request.
type route struct {
	reg *registration
	req intakerouting.Request
	// site is the Datadog site of the default host, or "" if the default host is not under an
	// official Datadog domain. Such a route is never routed.
	site string

	// Guarded by router.mu.

	// group is the group the route belongs to, or nil while its API key is not ready.
	group *group
	// current is the endpoint the route reports: the override when overridden, else the default.
	current    intakerouting.Endpoint
	overridden bool
	// decided is true once the route has its first decision.
	decided bool
	// keyCheckAt is when to read the API key again, to pick up a key that becomes ready or is
	// rotated.
	keyCheckAt time.Time
	// pendingCached is true when the route joined a group that already has an answer, and that
	// answer must be applied to it on the next pass.
	pendingCached bool
	// indirect is why the Agent did not reach the default host directly at the last check, or "".
	// It limits the logs to one per change.
	indirect string
}

func (r *router) Register(reqs []intakerouting.Request) intakerouting.Registration {
	now := r.clock.Now()
	reg := &registration{
		router:      r,
		routes:      make([]*route, len(reqs)),
		deadline:    now.Add(r.grace),
		changes:     make(chan struct{}, 1),
		initialDone: make(chan struct{}),
		closedCh:    make(chan struct{}),
	}
	for i, req := range reqs {
		reg.routes[i] = &route{reg: reg, req: req, site: siteOf(req.Default.Host), current: req.Default}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		for _, rt := range reg.routes {
			rt.decided = true
		}
		r.checkInitialLocked(reg)
		return reg
	}

	r.regs[reg] = struct{}{}
	for _, rt := range reg.routes {
		r.types[rt.req.Type] = struct{}{}
		if rt.site == "" {
			rt.decided = true
			continue
		}
		r.attachLocked(rt, now)
	}
	r.checkInitialLocked(reg)

	if !r.loopStarted {
		r.loopStarted = true
		go r.loop()
	}
	r.wakeLoop()
	return reg
}

// stop is the OnStop hook. It aborts in-flight requests and waits for the loop to exit.
func (r *router) stop(ctx context.Context) error {
	r.mu.Lock()
	r.stopped = true
	started := r.loopStarted
	r.mu.Unlock()

	r.cancel()
	if !started {
		return nil
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *router) wakeLoop() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (reg *registration) WaitInitial(ctx context.Context) []intakerouting.Endpoint {
	if wait := reg.deadline.Sub(reg.router.clock.Now()); wait > 0 {
		timer := reg.router.clock.Timer(wait)
		defer timer.Stop()
		select {
		case <-reg.initialDone:
		case <-reg.closedCh:
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	return reg.Current()
}

func (reg *registration) Current() []intakerouting.Endpoint {
	reg.router.mu.Lock()
	defer reg.router.mu.Unlock()
	endpoints := make([]intakerouting.Endpoint, len(reg.routes))
	for i, rt := range reg.routes {
		endpoints[i] = rt.current
	}
	return endpoints
}

func (reg *registration) Changes() <-chan struct{} {
	return reg.changes
}

func (reg *registration) Close() {
	r := reg.router
	r.mu.Lock()
	defer r.mu.Unlock()
	if reg.closed {
		return
	}
	reg.closed = true
	close(reg.closedCh)
	delete(r.regs, reg)
	for _, rt := range reg.routes {
		r.detachLocked(rt)
	}
	r.updateActiveOverridesLocked()
}

// notifyLocked signals a change without blocking. Signals are coalesced.
func (reg *registration) notifyLocked() {
	if reg.closed {
		return
	}
	select {
	case reg.changes <- struct{}{}:
	default:
	}
}

// checkInitialLocked releases WaitInitial once every route of reg has its first decision.
func (r *router) checkInitialLocked(reg *registration) {
	if reg.initialClosed {
		return
	}
	for _, rt := range reg.routes {
		if !rt.decided {
			return
		}
	}
	reg.initialClosed = true
	close(reg.initialDone)
}

// updateActiveOverridesLocked sets the active_overrides gauge of every telemetry type.
func (r *router) updateActiveOverridesLocked() {
	counts := make(map[intakerouting.TelemetryType]int, len(r.types))
	for reg := range r.regs {
		for _, rt := range reg.routes {
			if rt.overridden {
				counts[rt.req.Type]++
			}
		}
	}
	for t := range r.types {
		r.tlm.activeOverrides.Set(float64(counts[t]), string(t))
	}
}
