// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultKillActionFlushDelay = 2 * time.Second
	disarmerCacheFlushInterval  = 5 * time.Second
)

// ProcessKiller defines a process killer structure
type ProcessKiller struct {
	sync.Mutex

	cfg *config.Config

	enabled          bool
	pendingReports   []*KillActionReport
	binariesExcluded []*eval.Glob
	sourceAllowed    []string

	useDisarmers      *atomic.Bool
	disarmerStateCh   chan disarmerState
	ruleDisarmersLock sync.Mutex
	ruleDisarmers     map[rules.RuleID]*ruleDisarmer

	perRuleStatsLock sync.Mutex
	perRuleStats     map[rules.RuleID]*processKillerStats
}

type processKillerStats struct {
	processesKilled int64
}

// NewProcessKiller returns a new ProcessKiller
func NewProcessKiller(cfg *config.Config) (*ProcessKiller, error) {
	p := &ProcessKiller{
		cfg:             cfg,
		enabled:         true,
		useDisarmers:    atomic.NewBool(false),
		disarmerStateCh: make(chan disarmerState, 1),
		ruleDisarmers:   make(map[rules.RuleID]*ruleDisarmer),
		sourceAllowed:   cfg.RuntimeSecurity.EnforcementRuleSourceAllowed,
		perRuleStats:    make(map[rules.RuleID]*processKillerStats),
	}

	binaries := append(binariesExcluded, cfg.RuntimeSecurity.EnforcementBinaryExcluded...)

	for _, str := range binaries {
		glob, err := eval.NewGlob(str, false, false)
		if err != nil {
			return nil, err
		}

		p.binariesExcluded = append(p.binariesExcluded, glob)
	}

	return p, nil
}

// SetState sets the state - enabled or disabled - for the process killer
func (p *ProcessKiller) SetState(enabled bool) {
	p.Lock()
	defer p.Unlock()

	p.enabled = enabled
}

// AddPendingReports add a pending reports
func (p *ProcessKiller) AddPendingReports(report *KillActionReport) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = append(p.pendingReports, report)
}

// FlushPendingReports flush pending reports
func (p *ProcessKiller) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if time.Now().After(report.KilledAt.Add(defaultKillActionFlushDelay)) {
			report.resolved = true
			return true
		}
		return false
	})
}

// HandleProcessExited handles process exited events
func (p *ProcessKiller) HandleProcessExited(event *model.Event) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if report.Pid == event.ProcessContext.Pid {
			report.ExitedAt = event.ProcessContext.ExitTime
			report.resolved = true
			return true
		}
		return false
	})
}

func (p *ProcessKiller) isKillAllowed(pids []uint32, paths []string) (bool, error) {
	p.Lock()
	if !p.enabled {
		p.Unlock()
		return false, fmt.Errorf("the enforcement capability is disabled")
	}
	p.Unlock()

	for i, pid := range pids {
		if pid <= 1 || pid == utils.Getpid() {
			return false, fmt.Errorf("process with pid %d cannot be killed", pid)
		}

		if slices.ContainsFunc(p.binariesExcluded, func(glob *eval.Glob) bool {
			return glob.Matches(paths[i])
		}) {
			return false, fmt.Errorf("process `%s`(%d) is protected", paths[i], pid)
		}
	}
	return true, nil
}

func (p *ProcessKiller) isRuleAllowed(rule *rules.Rule) bool {
	return slices.Contains(p.sourceAllowed, rule.Policy.Source)
}

// KillAndReport kill and report, returns true if we did try to kill
func (p *ProcessKiller) KillAndReport(kill *rules.KillDefinition, rule *rules.Rule, ev *model.Event, killFnc func(pid uint32, sig uint32) error) bool {
	if !p.isRuleAllowed(rule) {
		log.Warnf("unable to kill, the source is not allowed: %v", rule)
		return false
	}

	entry, exists := ev.ResolveProcessCacheEntry()
	if !exists {
		return false
	}

	scope := "process"
	switch kill.Scope {
	case "container", "process":
		scope = kill.Scope
	}

	if p.useDisarmers.Load() {
		var disarmer *ruleDisarmer
		p.ruleDisarmersLock.Lock()
		if disarmer = p.ruleDisarmers[rule.ID]; disarmer == nil {
			containerParams, executableParams := p.getDisarmerParams(kill)
			disarmer = newRuleDisarmer(containerParams, executableParams)
			p.ruleDisarmers[rule.ID] = disarmer
		}
		p.ruleDisarmersLock.Unlock()

		onActionBlockedByDisarmer := func(dt disarmerType) {
			seclog.Warnf("skipping kill action of rule `%s` because it has been disarmed", rule.ID)
			ev.ActionReports = append(ev.ActionReports, &KillActionReport{
				Scope:        scope,
				Signal:       kill.Signal,
				Status:       KillActionStatusRuleDisarmed,
				DisarmerType: string(dt),
				CreatedAt:    ev.ProcessContext.ExecTime,
				DetectedAt:   ev.ResolveEventTime(),
				Pid:          ev.ProcessContext.Pid,
				rule:         rule,
			})
		}

		if disarmer.container.enabled {
			if containerID := ev.FieldHandlers.ResolveContainerID(ev, ev.ContainerContext); containerID != "" {
				if !disarmer.allow(disarmer.containerCache, containerID, func() {
					disarmer.disarmedCount[containerDisarmerType]++
					seclog.Warnf("disarming kill action of rule `%s` because more than %d different containers triggered it in the last %s", rule.ID, disarmer.container.capacity, disarmer.container.period)
				}) {
					onActionBlockedByDisarmer(containerDisarmerType)
					return false
				}
			}
		}

		if disarmer.executable.enabled {
			executable := entry.Process.FileEvent.PathnameStr
			if !disarmer.allow(disarmer.executableCache, executable, func() {
				disarmer.disarmedCount[executableDisarmerType]++
				seclog.Warnf("disarmed kill action of rule `%s` because more than %d different executables triggered it in the last %s", rule.ID, disarmer.executable.capacity, disarmer.executable.period)
			}) {
				onActionBlockedByDisarmer(executableDisarmerType)
				return false
			}
		}
	}

	pids, paths, err := p.getProcesses(scope, ev, entry)
	if err != nil {
		log.Errorf("unable to kill: %s", err)
		return false
	}

	// if one pids is not allowed don't kill anything
	if killAllowed, err := p.isKillAllowed(pids, paths); !killAllowed {
		log.Warnf("unable to kill: %v", err)
		return false
	}

	sig := model.SignalConstants[kill.Signal]

	var processesKilled int64
	killedAt := time.Now()
	for _, pid := range pids {
		log.Debugf("requesting signal %s to be sent to %d", kill.Signal, pid)

		if err := killFnc(uint32(pid), uint32(sig)); err != nil {
			seclog.Debugf("failed to kill process %d: %s", pid, err)
		} else {
			processesKilled++
		}
	}

	p.perRuleStatsLock.Lock()
	var stats *processKillerStats
	if stats = p.perRuleStats[rule.ID]; stats == nil {
		stats = &processKillerStats{}
		p.perRuleStats[rule.ID] = stats
	}
	stats.processesKilled += processesKilled
	p.perRuleStatsLock.Unlock()

	p.Lock()
	defer p.Unlock()

	report := &KillActionReport{
		Scope:      scope,
		Signal:     kill.Signal,
		Status:     KillActionStatusPerformed,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		KilledAt:   killedAt,
		Pid:        ev.ProcessContext.Pid,
		rule:       rule,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)

	return true
}

// Reset the state and statistics of the process killer
func (p *ProcessKiller) Reset(rs *rules.RuleSet) {
	if p.cfg.RuntimeSecurity.EnforcementEnabled {
		var ruleSetHasKillAction bool
		var rulesetHasKillDisarmer bool

	rules:
		for _, rule := range rs.GetRules() {
			if !p.isRuleAllowed(rule) {
				continue
			}
			for _, action := range rule.Actions {
				if action.Def.Kill == nil {
					continue
				}
				ruleSetHasKillAction = true
				if action.Def.Kill.Disarmer != nil && (action.Def.Kill.Disarmer.Container != nil || action.Def.Kill.Disarmer.Executable != nil) {
					rulesetHasKillDisarmer = true
					break rules
				}
			}
		}

		configHasKillDisarmer := p.cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled || p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled
		if ruleSetHasKillAction && (configHasKillDisarmer || rulesetHasKillDisarmer) {
			p.useDisarmers.Store(true)
			p.disarmerStateCh <- running
		} else {
			p.useDisarmers.Store(false)
			p.disarmerStateCh <- stopped
		}
	}

	p.perRuleStatsLock.Lock()
	clear(p.perRuleStats)
	p.perRuleStatsLock.Unlock()
	p.ruleDisarmersLock.Lock()
	clear(p.ruleDisarmers)
	p.ruleDisarmersLock.Unlock()
}

// SendStats sends runtime security enforcement statistics to Datadog
func (p *ProcessKiller) SendStats(statsd statsd.ClientInterface) {
	p.perRuleStatsLock.Lock()
	for ruleID, stats := range p.perRuleStats {
		ruleIDTag := []string{
			"rule_id:" + string(ruleID),
		}

		if stats.processesKilled > 0 {
			_ = statsd.Count(metrics.MetricEnforcementProcessKilled, stats.processesKilled, ruleIDTag, 1)
			stats.processesKilled = 0
		}
	}
	p.perRuleStatsLock.Unlock()

	p.ruleDisarmersLock.Lock()
	for ruleID, disarmer := range p.ruleDisarmers {
		ruleIDTag := []string{
			"rule_id:" + string(ruleID),
		}

		disarmer.Lock()
		for disarmerType, count := range disarmer.disarmedCount {
			if count > 0 {
				tags := append([]string{"disarmer_type:" + string(disarmerType)}, ruleIDTag...)
				_ = statsd.Count(metrics.MetricEnforcementRuleDisarmed, count, tags, 1)
				disarmer.disarmedCount[disarmerType] = 0
			}
		}
		if disarmer.rearmedCount > 0 {
			_ = statsd.Count(metrics.MetricEnforcementRuleRearmed, disarmer.rearmedCount, ruleIDTag, 1)
			disarmer.rearmedCount = 0
		}
		disarmer.Unlock()
	}
	p.ruleDisarmersLock.Unlock()
}

// Start starts the go rountine responsible for flushing the disarmer caches
func (p *ProcessKiller) Start(ctx context.Context, wg *sync.WaitGroup) {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(disarmerCacheFlushInterval)
		defer ticker.Stop()
		state := stopped
		for {
			switch state {
			case stopped:
				select {
				case state = <-p.disarmerStateCh:
					if state == running {
						ticker.Reset(disarmerCacheFlushInterval)
					}
					break
				case <-ctx.Done():
					return
				}
			case running:
				select {
				case state = <-p.disarmerStateCh:
					if state == stopped {
						ticker.Stop()
					}
					break
				case <-ctx.Done():
					return
				case <-ticker.C:
					p.ruleDisarmersLock.Lock()
					for ruleID, disarmer := range p.ruleDisarmers {
						disarmer.Lock()
						var cLength, eLength int
						if disarmer.container.enabled {
							cLength = disarmer.containerCache.flush()
						}
						if disarmer.executable.enabled {
							eLength = disarmer.executableCache.flush()
						}
						if disarmer.disarmed && cLength == 0 && eLength == 0 {
							disarmer.disarmed = false
							disarmer.rearmedCount++
							seclog.Infof("kill action of rule `%s` has been re-armed", ruleID)
						}
						disarmer.Unlock()
					}
					p.ruleDisarmersLock.Unlock()
				}
			}
		}
	}()
}

func (p *ProcessKiller) getDisarmerParams(kill *rules.KillDefinition) (*disarmerParams, *disarmerParams) {
	var containerParams, executableParams disarmerParams

	if kill.Disarmer != nil && kill.Disarmer.Container != nil && kill.Disarmer.Container.MaxAllowed > 0 {
		containerParams.enabled = true
		containerParams.capacity = uint64(kill.Disarmer.Container.MaxAllowed)
		containerParams.period = kill.Disarmer.Container.Period
	} else if p.cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled {
		containerParams.enabled = true
		containerParams.capacity = uint64(p.cfg.RuntimeSecurity.EnforcementDisarmerContainerMaxAllowed)
		containerParams.period = p.cfg.RuntimeSecurity.EnforcementDisarmerContainerPeriod
	}

	if kill.Disarmer != nil && kill.Disarmer.Executable != nil && kill.Disarmer.Executable.MaxAllowed > 0 {
		executableParams.enabled = true
		executableParams.capacity = uint64(kill.Disarmer.Executable.MaxAllowed)
		executableParams.period = kill.Disarmer.Executable.Period
	} else if p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled {
		executableParams.enabled = true
		executableParams.capacity = uint64(p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableMaxAllowed)
		executableParams.period = p.cfg.RuntimeSecurity.EnforcementDisarmerExecutablePeriod
	}

	return &containerParams, &executableParams
}

type disarmerState int

const (
	stopped disarmerState = iota
	running
)

type disarmerType string

const (
	containerDisarmerType  disarmerType = "container"
	executableDisarmerType disarmerType = "executable"
)

type ruleDisarmer struct {
	sync.Mutex
	disarmed        bool
	container       disarmerParams
	containerCache  *disarmerCache[string, bool]
	executable      disarmerParams
	executableCache *disarmerCache[string, bool]
	// stats
	disarmedCount map[disarmerType]int64
	rearmedCount  int64
}

type disarmerParams struct {
	enabled  bool
	capacity uint64
	period   time.Duration
}

type disarmerCache[K comparable, V any] struct {
	*ttlcache.Cache[K, V]
	capacity uint64
}

func newDisarmerCache[K comparable, V any](params *disarmerParams) *disarmerCache[K, V] {
	cacheOpts := []ttlcache.Option[K, V]{
		ttlcache.WithCapacity[K, V](params.capacity),
	}

	if params.period > 0 {
		cacheOpts = append(cacheOpts, ttlcache.WithTTL[K, V](params.period))
	}

	return &disarmerCache[K, V]{
		Cache:    ttlcache.New(cacheOpts...),
		capacity: params.capacity,
	}
}

func (c *disarmerCache[K, V]) flush() int {
	c.DeleteExpired()
	return c.Len()
}

func newRuleDisarmer(containerParams *disarmerParams, executableParams *disarmerParams) *ruleDisarmer {
	kd := &ruleDisarmer{
		disarmed:      false,
		container:     *containerParams,
		executable:    *executableParams,
		disarmedCount: make(map[disarmerType]int64),
	}

	if kd.container.enabled {
		kd.containerCache = newDisarmerCache[string, bool](containerParams)
	}

	if kd.executable.enabled {
		kd.executableCache = newDisarmerCache[string, bool](executableParams)
	}

	return kd
}

func (rd *ruleDisarmer) allow(cache *disarmerCache[string, bool], key string, onDisarm func()) bool {
	rd.Lock()
	defer rd.Unlock()

	if cache == nil {
		return true
	}

	cache.DeleteExpired()
	// if the key is not in the cache, check if the new key causes the number of keys to exceed the capacity
	// otherwise, the key is already in the cache and cache.Get will update its TTL
	if cache.Get(key) == nil {
		alreadyAtCapacity := uint64(cache.Len()) >= cache.capacity
		cache.Set(key, true, ttlcache.DefaultTTL)
		if alreadyAtCapacity && !rd.disarmed {
			rd.disarmed = true
			onDisarm()
		}
	}

	return !rd.disarmed
}
