// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	"context"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v7"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

const (
	queryTimeout       = 10 * time.Second
	preflightTimeout   = 5 * time.Second
	pathCheckTimeout   = 5 * time.Second
	keyRecheckInterval = 5 * time.Second
	refreshJitter      = 0.1

	// Until a group gets its first answer, failed queries are retried with this backoff.
	startupBackoffInitial       = time.Second
	startupBackoffMultiplier    = 2
	startupBackoffMax           = 5 * time.Minute
	startupBackoffRandomization = 0.2
)

type groupKey struct {
	site   string
	apiKey string
}

// group is the set of routes that share a site and an API key, and so share a query.
type group struct {
	key    groupKey
	routes map[*route]struct{}
	// answered is true once a query was settled. Until then, failed queries are retried with the
	// startup backoff instead of waiting for the next refresh.
	answered bool
	// cached is the last settled result. It is applied to routes that join the group later.
	cached      *queryResult
	nextAttempt time.Time
	backoff     *backoff.ExponentialBackOff
	// failing is true when the last query failed. It limits failure warnings to one per streak.
	failing bool
}

// routeSnapshot is what the unlocked phases of a pass need to know about a route.
type routeSnapshot struct {
	route   *route
	typ     intakerouting.TelemetryType
	def     intakerouting.Endpoint
	current intakerouting.Endpoint
	// indirect is why the Agent does not reach def directly (a path reason), or "". indirectErr
	// describes it. They are set by checkPaths.
	indirect    string
	indirectErr error
}

// groupJob is the work of a pass for one group: a fresh query, or applying the cached answer to
// routes that joined the group since it was received.
type groupJob struct {
	group *group
	key   groupKey
	fresh bool
	// skipped is set on a fresh job whose query is not sent, because no route of the group may be
	// routed.
	skipped bool
	routes  []routeSnapshot
	result  queryResult
}

type decisionKind int

const (
	keepCurrent decisionKind = iota
	useDefault
	useOverride
)

type decision struct {
	kind   decisionKind
	target intakerouting.Endpoint
	// probe is set while an override waits for its pre-flight check.
	probe *probeKey
}

// probeKey identifies a pre-flight check. A pass checks each host, port and key at most once.
type probeKey struct {
	host   string
	port   int
	apiKey string
}

// loop runs passes until the component stops. A pass runs when one is due, and when a
// registration asks for it.
func (r *router) loop() {
	defer close(r.done)
	for {
		next := r.runPass()
		if r.ctx.Err() != nil {
			return
		}
		now := r.clock.Now()
		if !next.IsZero() && !next.After(now) {
			continue
		}
		if !r.wait(next.Sub(now), next.IsZero()) {
			return
		}
	}
}

// wait blocks until d elapses (forever if indefinitely is set), a pass is requested, or the
// component stops. It returns false in the latter case.
func (r *router) wait(d time.Duration, indefinitely bool) bool {
	var timerC <-chan time.Time
	if !indefinitely {
		timer := r.clock.Timer(d)
		defer timer.Stop()
		timerC = timer.C
	}
	if r.afterPass != nil {
		r.afterPass()
	}
	select {
	case <-r.ctx.Done():
		return false
	case <-r.wake:
	case <-timerC:
	}
	return true
}

// runPass collects the groups to query and the routes to update (locked), checks how the default
// hosts are reached and sends the queries (unlocked), validates the answers and runs pre-flight
// checks (unlocked), then applies the decisions (locked). It returns when the next pass is due, or
// the zero time if nothing is scheduled.
func (r *router) runPass() time.Time {
	jobs, next := r.collect()
	if len(jobs) == 0 {
		return next
	}
	r.checkPaths(jobs)
	r.queryAll(jobs)
	decisions := r.evaluate(jobs)
	return r.apply(jobs, decisions)
}

// collect is the first phase of a pass.
func (r *router) collect() ([]*groupJob, time.Time) {
	now := r.clock.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil, time.Time{}
	}

	// Pick up API keys that became ready or were rotated.
	for reg := range r.regs {
		for _, rt := range reg.routes {
			if rt.site != "" {
				r.attachLocked(rt, now)
			}
		}
		r.checkInitialLocked(reg)
	}

	var jobs []*groupJob
	for key, g := range r.groups {
		if !g.nextAttempt.After(now) {
			jobs = append(jobs, &groupJob{group: g, key: key, fresh: true, routes: g.snapshotLocked(false)})
		} else if g.cached != nil {
			if routes := g.snapshotLocked(true); len(routes) > 0 {
				jobs = append(jobs, &groupJob{group: g, key: key, routes: routes, result: *g.cached})
			}
		}
	}
	if len(jobs) > 0 {
		return jobs, time.Time{}
	}
	return nil, r.nextDueLocked(now)
}

// checkPaths is the second phase of a pass: it checks, in parallel, how the Agent reaches the
// default host of every route. Another host would not be reached the same way: behind PrivateLink,
// it would send telemetry over the internet instead, at a much higher cost. So a route whose default
// host resolves to a non-public address, or is proxied, is never routed, and a fresh query is not
// sent when no route of its group may be routed.
func (r *router) checkPaths(jobs []*groupJob) {
	type pathResult struct {
		reason string
		err    error
	}
	// Hosts are checked as configured: that is how the sender matches them against no_proxy.
	results := make(map[intakerouting.Endpoint]pathResult)
	for _, job := range jobs {
		for _, s := range job.routes {
			results[s.def] = pathResult{}
		}
	}
	endpoints := make([]intakerouting.Endpoint, 0, len(results))
	for ep := range results {
		endpoints = append(endpoints, ep)
	}
	checked := make([]pathResult, len(endpoints))
	var wg sync.WaitGroup
	for i, ep := range endpoints {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(r.ctx, pathCheckTimeout)
			defer cancel()
			reason, err := r.paths.check(ctx, ep)
			checked[i] = pathResult{reason: reason, err: err}
		})
	}
	wg.Wait()
	for i, ep := range endpoints {
		results[ep] = checked[i]
	}

	for _, job := range jobs {
		// A lookup failure does not skip the query: a failing query retries with the startup
		// backoff, so the group is answered soon after DNS works again.
		mayRoute := false
		reported := make(map[[2]string]bool)
		for j := range job.routes {
			s := &job.routes[j]
			res := results[s.def]
			s.indirect, s.indirectErr = res.reason, res.err
			if s.indirect == "" || s.indirect == pathLookupFailed {
				mayRoute = true
			}
			// Counted once per telemetry type and reason, like rejected answers.
			if key := [2]string{string(s.typ), s.indirect}; s.indirect != "" && job.fresh && !reported[key] && r.ctx.Err() == nil {
				reported[key] = true
				r.tlm.skippedEndpoints.Inc(string(s.typ), s.indirect)
			}
		}
		if job.fresh && !mayRoute {
			job.skipped = true
			job.result = queryResult{status: statusSkipped}
		}
	}
}

// queryAll is the third phase of a pass: it sends the fresh queries in parallel.
func (r *router) queryAll(jobs []*groupJob) {
	var wg sync.WaitGroup
	for _, job := range jobs {
		if !job.fresh || job.skipped {
			continue
		}
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(r.ctx, queryTimeout)
			defer cancel()
			job.result = r.client.query(ctx, job.key.site, job.key.apiKey)
			if r.ctx.Err() == nil {
				r.tlm.queries.Inc(job.result.status)
			}
		})
	}
	wg.Wait()
}

// evaluate is the fourth phase of a pass: it turns answers into decisions, and runs the
// pre-flight checks of the overrides that would switch an endpoint.
func (r *router) evaluate(jobs []*groupJob) [][]decision {
	decisions := make([][]decision, len(jobs))
	probes := make(map[probeKey]bool)
	for i, job := range jobs {
		decisions[i] = make([]decision, len(job.routes))
		// Whatever the answer, a route whose default host is not reached directly goes back to
		// it, and one whose default host could not be resolved keeps its current endpoint.
		for j, s := range job.routes {
			if s.indirect != "" && s.indirect != pathLookupFailed {
				decisions[i][j] = decision{kind: useDefault}
			}
		}
		if job.result.status != statusSuccess {
			continue
		}
		// A fresh answer is counted and logged once per telemetry type.
		reported := make(map[intakerouting.TelemetryType]bool)
		for j, s := range job.routes {
			if s.indirect != "" {
				continue
			}
			d, reason, err := decide(job, s)
			if reason != "" && job.fresh && !reported[s.typ] {
				reported[s.typ] = true
				r.tlm.rejectedAnswers.Inc(string(s.typ), reason)
				if reason == rejectInvalidHost {
					r.log.Warnf("Intake routing: ignoring the %s endpoint returned for site %s, keeping the current endpoint: %v", s.typ, job.key.site, err)
				} else {
					r.log.Debugf("Intake routing: the answer for site %s has no %s entry, keeping the current endpoint", job.key.site, s.typ)
				}
			}
			if d.probe != nil {
				probes[*d.probe] = false
			}
			decisions[i][j] = d
		}
	}

	r.runPreflights(probes)
	for i := range decisions {
		for j := range decisions[i] {
			d := &decisions[i][j]
			if d.probe == nil {
				continue
			}
			if !probes[*d.probe] {
				d.kind = keepCurrent
			}
			d.probe = nil
		}
	}
	return decisions
}

// decide applies the decision table to one route. When the answer is not applied, it returns the
// reason (a rejected_answers tag value), and the validation error for an invalid host.
func decide(job *groupJob, s routeSnapshot) (decision, string, error) {
	answer, ok := job.result.routes[s.typ]
	if !ok {
		return decision{kind: keepCurrent}, rejectAbsent, nil
	}
	if !answer.override {
		return decision{kind: useDefault}, "", nil
	}
	target := intakerouting.Endpoint{Host: answer.host, Port: normalizePort(answer.port)}
	if sameEndpoint(target, s.def) {
		return decision{kind: useDefault}, "", nil
	}
	if err := validateOverride(answer.host, answer.port); err != nil {
		return decision{kind: keepCurrent}, rejectInvalidHost, err
	}
	if sameEndpoint(target, s.current) {
		return decision{kind: keepCurrent}, "", nil
	}
	return decision{
		kind:   useOverride,
		target: target,
		probe:  &probeKey{host: target.Host, port: target.Port, apiKey: job.key.apiKey},
	}, "", nil
}

// runPreflights runs the given pre-flight checks in parallel and records whether each passed.
func (r *router) runPreflights(probes map[probeKey]bool) {
	keys := make([]probeKey, 0, len(probes))
	for pk := range probes {
		keys = append(keys, pk)
	}
	passed := make([]bool, len(keys))
	var wg sync.WaitGroup
	for i, pk := range keys {
		wg.Go(func() {
			ep := intakerouting.Endpoint{Host: pk.host, Port: pk.port}
			ctx, cancel := context.WithTimeout(r.ctx, preflightTimeout)
			defer cancel()
			err := r.client.preflight(ctx, ep, pk.apiKey)
			if r.ctx.Err() != nil {
				return
			}
			if err != nil {
				r.tlm.preflights.Inc(preflightFailure)
				r.log.Warnf("Intake routing: pre-flight check of %s failed, keeping the current endpoint: %v", hostPort(ep), err)
				return
			}
			r.tlm.preflights.Inc(preflightSuccess)
			passed[i] = true
		})
	}
	wg.Wait()
	for i, pk := range keys {
		probes[pk] = passed[i]
	}
}

// apply is the last phase of a pass: it updates the routes, notifies their registrations and
// schedules the next queries.
func (r *router) apply(jobs []*groupJob, decisions [][]decision) time.Time {
	now := r.clock.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return time.Time{}
	}

	changed := make(map[*registration]struct{})
	for i, job := range jobs {
		g := job.group
		if r.groups[job.key] != g {
			// Every route left the group during the pass.
			continue
		}
		if job.fresh {
			r.scheduleLocked(g, job.result, now)
		}
		applied := make(map[*route]struct{}, len(job.routes))
		for j, s := range job.routes {
			rt := s.route
			if rt.group != g {
				continue
			}
			applied[rt] = struct{}{}
			rt.pendingCached = false
			rt.decided = true
			r.logPathLocked(rt, s)
			if r.applyLocked(rt, decisions[i][j]) {
				changed[rt.reg] = struct{}{}
			}
		}
		if !job.fresh {
			continue
		}
		// Routes that joined the group during the query get its answer on the next pass.
		for rt := range g.routes {
			if _, ok := applied[rt]; ok {
				continue
			}
			if g.cached != nil {
				rt.pendingCached = true
			} else {
				rt.decided = true
			}
		}
	}

	for reg := range r.regs {
		r.checkInitialLocked(reg)
	}
	for reg := range changed {
		reg.notifyLocked()
	}
	r.updateActiveOverridesLocked()
	return r.nextDueLocked(now)
}

// logPathLocked logs when the Agent stops or starts reaching the default host of a route
// directly.
func (r *router) logPathLocked(rt *route, s routeSnapshot) {
	if s.indirect == rt.indirect {
		return
	}
	switch {
	case s.indirect == pathLookupFailed:
		r.log.Warnf("Intake routing: keeping %s on its current host, the Agent could not resolve its configured host: %v", rt.req.Type, s.indirectErr)
	case s.indirect != "":
		r.log.Infof("Intake routing: keeping %s on its configured host, which the Agent does not reach directly (for example through PrivateLink): %v", rt.req.Type, s.indirectErr)
	default:
		r.log.Infof("Intake routing: the Agent reaches the configured host of %s directly again, it may be routed", rt.req.Type)
	}
	rt.indirect = s.indirect
}

// scheduleLocked records the result of a group's query and schedules the next one.
func (r *router) scheduleLocked(g *group, res queryResult, now time.Time) {
	if res.settled() {
		if g.failing && res.status != statusSkipped {
			r.log.Infof("Intake routing: queries for site %s succeed again", g.key.site)
		}
		g.failing = false
		g.answered = true
		g.cached = &res
		g.backoff.Reset()
		g.nextAttempt = now.Add(r.jitter(r.refresh))
		return
	}

	switch {
	case g.failing:
		r.log.Debugf("Intake routing: query for site %s failed again (%s), keeping the current endpoints: %v", g.key.site, res.status, res.err)
	case res.code == 401 || res.code == 403:
		r.log.Warnf("Intake routing: the Datadog backend of site %s rejected the API key (HTTP %d), keeping the current endpoints", g.key.site, res.code)
	default:
		r.log.Warnf("Intake routing: query for site %s failed (%s), keeping the current endpoints: %v", g.key.site, res.status, res.err)
	}
	g.failing = true
	if g.answered {
		g.nextAttempt = now.Add(r.jitter(r.refresh))
	} else {
		g.nextAttempt = now.Add(g.backoff.NextBackOff())
	}
}

// applyLocked applies a decision to a route and reports whether its endpoint changed.
func (r *router) applyLocked(rt *route, d decision) bool {
	var target intakerouting.Endpoint
	var overridden bool
	switch d.kind {
	case useDefault:
		target = rt.req.Default
	case useOverride:
		target, overridden = d.target, true
	default:
		return false
	}
	if rt.overridden == overridden && rt.current == target {
		return false
	}

	direction := directionToDefault
	switch {
	case overridden && rt.overridden:
		direction = directionEnhancedToEnhanced
	case overridden:
		direction = directionToEnhanced
	}
	r.tlm.switches.Inc(string(rt.req.Type), direction)
	if overridden {
		r.log.Infof("Intake routing: sending %s to %s instead of its configured host %s", rt.req.Type, hostPort(target), hostPort(rt.req.Default))
	} else {
		r.log.Infof("Intake routing: sending %s to its configured host %s again", rt.req.Type, hostPort(rt.req.Default))
	}
	rt.current, rt.overridden = target, overridden
	return true
}

// attachLocked reads the API key of a route and moves the route to the group of its site and key.
// A route whose key is not ready leaves its group and keeps its current endpoint. The key is read
// again within keyRecheck, so a rotation is picked up without waiting for the next refresh.
func (r *router) attachLocked(rt *route, now time.Time) {
	rt.keyCheckAt = now.Add(r.keyRecheck)
	var apiKey string
	var ready bool
	if rt.req.APIKey != nil {
		apiKey, ready = rt.req.APIKey()
	}
	if !ready || apiKey == "" {
		r.detachLocked(rt)
		rt.decided = true
		return
	}

	key := groupKey{site: rt.site, apiKey: apiKey}
	if rt.group != nil && rt.group.key == key {
		return
	}
	r.detachLocked(rt)
	g, ok := r.groups[key]
	if !ok {
		g = &group{key: key, routes: make(map[*route]struct{}), nextAttempt: now, backoff: r.newStartupBackoff()}
		r.groups[key] = g
	}
	g.routes[rt] = struct{}{}
	rt.group = g
	switch {
	case g.cached != nil:
		rt.pendingCached = true
	case g.failing:
		// The group's queries are failing: the route keeps its endpoint until the next attempt.
		rt.decided = true
	}
}

// detachLocked removes a route from its group, and drops the group once it is empty.
func (r *router) detachLocked(rt *route) {
	g := rt.group
	if g == nil {
		return
	}
	delete(g.routes, rt)
	if len(g.routes) == 0 {
		delete(r.groups, g.key)
	}
	rt.group = nil
	rt.pendingCached = false
}

// nextDueLocked returns when the next pass is due, or the zero time if nothing is scheduled.
func (r *router) nextDueLocked(now time.Time) time.Time {
	var next time.Time
	consider := func(t time.Time) {
		if next.IsZero() || t.Before(next) {
			next = t
		}
	}
	for _, g := range r.groups {
		consider(g.nextAttempt)
		if g.cached == nil {
			continue
		}
		for rt := range g.routes {
			if rt.pendingCached {
				consider(now)
				break
			}
		}
	}
	for reg := range r.regs {
		for _, rt := range reg.routes {
			if rt.site != "" {
				consider(rt.keyCheckAt)
			}
		}
	}
	return next
}

func (r *router) newStartupBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = startupBackoffInitial
	b.Multiplier = startupBackoffMultiplier
	b.MaxInterval = startupBackoffMax
	b.RandomizationFactor = r.startupBackoffRandomization
	b.Reset()
	return b
}

// snapshotLocked returns the group's routes, or only those waiting for the cached answer.
func (g *group) snapshotLocked(pendingOnly bool) []routeSnapshot {
	routes := make([]routeSnapshot, 0, len(g.routes))
	for rt := range g.routes {
		if pendingOnly && !rt.pendingCached {
			continue
		}
		routes = append(routes, routeSnapshot{route: rt, typ: rt.req.Type, def: rt.req.Default, current: rt.current})
	}
	return routes
}
