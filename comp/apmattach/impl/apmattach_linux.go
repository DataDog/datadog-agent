// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

const (
	cfgPrefix       = "apm_config.instrumentation.attach."
	cfgEnabled      = cfgPrefix + "enabled"
	cfgScanInterval = cfgPrefix + "scan_interval"
	cfgInjectorPath = cfgPrefix + "injector_path"
	cfgTimeout      = cfgPrefix + "timeout"

	defaultScanInterval = 60 * time.Second
	defaultTimeout      = 30 * time.Second
	minDuration         = time.Second

	policyPollInterval = 5 * time.Second

	maxAttempts      = 3
	retryBaseBackoff = time.Minute
)

type processState struct {
	key                 procKey
	exe                 string
	result              helperResult
	consecutiveFailures int
	lastAttempt         time.Time
	nextRetry           time.Time
	evaluatedPolicyGen  uint64
}

type attacher struct {
	log          log.Component
	enabled      bool
	injectorPath string
	scanInterval time.Duration
	timeout      time.Duration
	procRoot     string

	helper   helperRunner
	policies *policyWatcher
	now      func() time.Time
	euid     int

	cancel context.CancelFunc
	done   chan struct{}

	// The scan loop is the only writer; the status provider reads under mu.
	mu                sync.Mutex
	policyGen         uint64
	states            map[procKey]*processState
	lastScan          time.Time
	lastPolicyChange  time.Time
	injectorAvailable bool
	lastError         string
	warnedNoInjector  bool
}

func newComponent(reqs Requires) (Provides, error) {
	a := newAttacher(reqs.Log, reqs.Config)
	if !a.enabled {
		return Provides{Comp: a, Status: status.NewInformationProvider(nil)}, nil
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: a.start, OnStop: a.stop})
	return Provides{
		Comp:   a,
		Status: status.NewInformationProvider(statusProvider{a: a}),
	}, nil
}

func newAttacher(logger log.Component, cfg config.Component) *attacher {
	a := &attacher{
		log:          logger,
		enabled:      cfg.GetBool(cfgEnabled),
		injectorPath: cfg.GetString(cfgInjectorPath),
		scanInterval: durationSetting(logger, cfg, cfgScanInterval, defaultScanInterval),
		timeout:      durationSetting(logger, cfg, cfgTimeout, defaultTimeout),
		procRoot:     "/proc",
		now:          time.Now,
		euid:         os.Geteuid(),
		states:       map[procKey]*processState{},
	}
	a.injectorAvailable = isExecutable(a.injectorPath)
	a.helper = &execHelper{path: a.injectorPath, timeout: a.timeout, grace: helperGracePeriod}
	a.policies = newPolicyWatcher(policyFiles(config.DefaultConfPath))
	return a
}

func durationSetting(logger log.Component, cfg config.Component, key string, def time.Duration) time.Duration {
	d := cfg.GetDuration(key)
	if d < minDuration {
		if cfg.IsConfigured(key) {
			logger.Warnf("APM attach: invalid %s %q (must be a duration of at least %s, such as 60s); using %s", key, cfg.GetString(key), minDuration, def)
		}
		return def
	}
	return d
}

func (a *attacher) start(_ context.Context) error {
	a.log.Infof("APM attach: enabled, scanning for running JVMs every %s with %s", a.scanInterval, a.injectorPath)
	if a.euid != 0 {
		a.log.Warnf("APM attach: the Agent runs as uid %d but the attach helper requires root; attaches fail unless the helper can elevate", a.euid)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.done = make(chan struct{})
	go a.loop(ctx)
	return nil
}

func (a *attacher) stop(_ context.Context) error {
	if a.cancel == nil {
		return nil
	}
	a.cancel()
	<-a.done
	return nil
}

func (a *attacher) loop(ctx context.Context) {
	defer close(a.done)

	scanTicker := time.NewTicker(a.scanInterval)
	defer scanTicker.Stop()
	policyTicker := time.NewTicker(policyPollInterval)
	defer policyTicker.Stop()

	a.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-scanTicker.C:
			a.scan(ctx)
		case <-policyTicker.C:
			if a.checkPolicies() {
				a.scan(ctx)
				scanTicker.Reset(a.scanInterval)
			}
		}
	}
}

func (a *attacher) checkPolicies() bool {
	if !a.policies.changed() {
		return false
	}
	a.mu.Lock()
	a.policyGen++
	a.lastPolicyChange = a.now()
	a.mu.Unlock()
	a.log.Infof("APM attach: workload selection policy changed, re-evaluating skipped processes")
	return true
}

func (a *attacher) scan(ctx context.Context) {
	injectorAvailable := isExecutable(a.injectorPath)
	candidates, err := listJavaProcesses(a.procRoot)
	now := a.now()
	if err != nil {
		a.log.Warnf("APM attach: could not list processes: %v", err)
		a.mu.Lock()
		a.lastScan, a.lastError, a.injectorAvailable = now, "could not list processes: "+err.Error(), injectorAvailable
		a.mu.Unlock()
		return
	}

	a.mu.Lock()
	a.lastScan = now
	a.lastError = ""
	a.injectorAvailable = injectorAvailable
	if !injectorAvailable {
		a.lastError = "attach helper not found or not executable: " + a.injectorPath
	}
	warn := !injectorAvailable && !a.warnedNoInjector
	if warn {
		a.warnedNoInjector = true
	} else if injectorAvailable {
		a.warnedNoInjector = false
	}
	live := make(map[procKey]struct{}, len(candidates))
	for _, c := range candidates {
		live[c.key] = struct{}{}
	}
	for key := range a.states {
		if _, ok := live[key]; !ok {
			delete(a.states, key)
		}
	}
	var todo []candidate
	for _, c := range candidates {
		if a.needsEvaluationLocked(a.states[c.key], now) {
			todo = append(todo, c)
		}
	}
	gen := a.policyGen
	a.mu.Unlock()

	if !injectorAvailable {
		if warn && len(todo) > 0 {
			a.log.Warnf("APM attach: %d JVM(s) found but the attach helper %s is missing or not executable", len(todo), a.injectorPath)
		}
		return
	}

	for _, c := range todo {
		if ctx.Err() != nil {
			return
		}
		res := a.helper.run(ctx, c.key.pid)
		if ctx.Err() != nil {
			return
		}
		if !a.stillRunning(c.key) {
			a.log.Debugf("APM attach: pid %d exited during evaluation", c.key.pid)
			a.mu.Lock()
			delete(a.states, c.key)
			a.mu.Unlock()
			continue
		}
		a.record(c, res, gen)
	}
}

func (a *attacher) needsEvaluationLocked(st *processState, now time.Time) bool {
	if st == nil {
		return true
	}
	switch st.result.Decision {
	case decisionAttached:
		return false
	case decisionFailed:
		return st.consecutiveFailures < maxAttempts && !now.Before(st.nextRetry)
	default:
		return st.evaluatedPolicyGen != a.policyGen
	}
}

func (a *attacher) record(c candidate, res helperResult, gen uint64) {
	now := a.now()
	a.mu.Lock()
	st := a.states[c.key]
	if st == nil {
		st = &processState{key: c.key, exe: c.exe}
		a.states[c.key] = st
	}
	st.result = res
	st.lastAttempt = now
	st.evaluatedPolicyGen = gen
	if res.Decision == decisionFailed {
		st.consecutiveFailures++
		st.nextRetry = now.Add(retryBackoff(st.consecutiveFailures))
	} else {
		st.consecutiveFailures = 0
		st.nextRetry = time.Time{}
	}
	failures := st.consecutiveFailures
	a.mu.Unlock()

	suffix := ""
	if res.Decision == decisionFailed {
		if failures < maxAttempts {
			suffix = fmt.Sprintf(" (attempt %d/%d, will retry)", failures, maxAttempts)
		} else {
			suffix = fmt.Sprintf(" (attempt %d/%d, giving up)", failures, maxAttempts)
		}
	}
	a.log.Infof("APM attach: pid=%d decision=%s reason=%s rule=%q layer=%q service=%q tracer_version=%q duration_ms=%d detail=%q%s",
		c.key.pid, res.Decision, res.Reason, res.Rule, res.Layer, res.Service, res.TracerVersion, res.DurationMs, res.Detail, suffix)
}

func (a *attacher) stillRunning(key procKey) bool {
	st, err := processStartTime(a.procRoot, key.pid)
	return err == nil && st == key.startTime
}

func retryBackoff(failures int) time.Duration {
	d := retryBaseBackoff
	for i := 1; i < failures; i++ {
		d *= 4
	}
	return d
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}
