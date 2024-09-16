// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"

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

	ruleDisarmersLock sync.Mutex
	ruleDisarmers     map[rules.RuleID]*killDisarmer

	perRuleStatsLock sync.Mutex
	perRuleStats     map[rules.RuleID]*processKillerStats
}

type processKillerStats struct {
	actionPerformed int64
	processesKilled int64
}

// NewProcessKiller returns a new ProcessKiller
func NewProcessKiller(cfg *config.Config) (*ProcessKiller, error) {
	p := &ProcessKiller{
		cfg:           cfg,
		enabled:       true,
		ruleDisarmers: make(map[rules.RuleID]*killDisarmer),
		sourceAllowed: cfg.RuntimeSecurity.EnforcementRuleSourceAllowed,
		perRuleStats:  make(map[rules.RuleID]*processKillerStats),
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

func (p *ProcessKiller) isKillAllowed(pids []uint32, paths []string) bool {
	p.Lock()
	if !p.enabled {
		p.Unlock()
		return false
	}
	p.Unlock()

	for i, pid := range pids {
		if pid <= 1 || pid == utils.Getpid() {
			return false
		}

		if slices.ContainsFunc(p.binariesExcluded, func(glob *eval.Glob) bool {
			return glob.Matches(paths[i])
		}) {
			return false
		}
	}
	return true
}

func (p *ProcessKiller) isRuleAllowed(rule *rules.Rule) bool {
	return slices.Contains(p.sourceAllowed, rule.Policy.Source)
}

// KillAndReport kill and report
func (p *ProcessKiller) KillAndReport(scope string, signal string, rule *rules.Rule, ev *model.Event, killFnc func(pid uint32, sig uint32) error) {
	if !p.isRuleAllowed(rule) {
		log.Warnf("unable to kill, the source is not allowed: %v", rule)
		return
	}

	entry, exists := ev.ResolveProcessCacheEntry()
	if !exists {
		return
	}

	rsConfig := p.cfg.RuntimeSecurity

	if rsConfig.EnforcementDisarmerContainerEnabled || rsConfig.EnforcementDisarmerExecutableEnabled {
		var disarmer *killDisarmer
		p.ruleDisarmersLock.Lock()
		if disarmer = p.ruleDisarmers[rule.ID]; disarmer == nil {
			disarmer = newKillDisarmer(rsConfig, rule.ID)
			p.ruleDisarmers[rule.ID] = disarmer
		}
		p.ruleDisarmersLock.Unlock()

		if rsConfig.EnforcementDisarmerContainerEnabled {
			if containerID := ev.FieldHandlers.ResolveContainerID(ev, ev.ContainerContext); containerID != "" {
				if !disarmer.allow(disarmer.containerCache, containerDisarmer, containerID, func() {
					seclog.Warnf("disarming kill action of rule `%s` because more than %d different containers triggered it in the last %s", rule.ID, disarmer.containerCache.capacity, rsConfig.EnforcementDisarmerContainerPeriod)
				}) {
					seclog.Warnf("skipping kill action of rule `%s` because it has been disarmed", rule.ID)
					return
				}
			}
		}

		if rsConfig.EnforcementDisarmerExecutableEnabled {
			executable := entry.Process.FileEvent.PathnameStr
			if !disarmer.allow(disarmer.executableCache, executableDisarmer, executable, func() {
				seclog.Warnf("disarmed kill action of rule `%s` because more than %d different executables triggered it in the last %s", rule.ID, disarmer.executableCache.capacity, rsConfig.EnforcementDisarmerExecutablePeriod)
			}) {
				seclog.Warnf("skipping kill action of rule `%s` because it has been disarmed", rule.ID)
				return
			}
		}
	}

	switch scope {
	case "container", "process":
	default:
		scope = "process"
	}

	pids, paths, err := p.getProcesses(scope, ev, entry)
	if err != nil {
		log.Errorf("unable to kill: %s", err)
		return
	}

	// if one pids is not allowed don't kill anything
	if !p.isKillAllowed(pids, paths) {
		log.Warnf("unable to kill, some processes are protected: %v, %v", pids, paths)
		return
	}

	sig := model.SignalConstants[signal]

	var processesKilled int64
	killedAt := time.Now()
	for _, pid := range pids {
		log.Debugf("requesting signal %s to be sent to %d", signal, pid)

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
	stats.actionPerformed++
	stats.processesKilled += processesKilled
	p.perRuleStatsLock.Unlock()

	p.Lock()
	defer p.Unlock()

	report := &KillActionReport{
		Scope:      scope,
		Signal:     signal,
		Pid:        ev.ProcessContext.Pid,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		KilledAt:   killedAt,
		rule:       rule,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)
}

// Reset resets the disarmer state
func (p *ProcessKiller) Reset() {
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

		if stats.actionPerformed > 0 {
			_ = statsd.Count(metrics.MetricEnforcementKillActionPerformed, stats.actionPerformed, ruleIDTag, 1)
			stats.actionPerformed = 0
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
	if !p.cfg.RuntimeSecurity.EnforcementEnabled || (!p.cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled && !p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled) {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(disarmerCacheFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.ruleDisarmersLock.Lock()
				for _, disarmer := range p.ruleDisarmers {
					disarmer.Lock()
					var cLength, eLength int
					if disarmer.containerCache != nil {
						cLength = disarmer.containerCache.flush()
					}
					if disarmer.executableCache != nil {
						eLength = disarmer.executableCache.flush()
					}
					if disarmer.disarmed && cLength == 0 && eLength == 0 {
						disarmer.disarmed = false
						disarmer.rearmedCount++
						seclog.Infof("kill action of rule `%s` has been re-armed", disarmer.ruleID)
					}
					disarmer.Unlock()
				}
				p.ruleDisarmersLock.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

type disarmerType string

const (
	containerDisarmer  = disarmerType("container")
	executableDisarmer = disarmerType("executable")
)

type killDisarmer struct {
	sync.Mutex
	disarmed        bool
	ruleID          rules.RuleID
	containerCache  *disarmerCache[string, bool]
	executableCache *disarmerCache[string, bool]
	// stats
	disarmedCount map[disarmerType]int64
	rearmedCount  int64
}

type disarmerCache[K comparable, V any] struct {
	*ttlcache.Cache[K, V]
	capacity uint64
}

func newDisarmerCache[K comparable, V any](capacity uint64, period time.Duration) *disarmerCache[K, V] {
	cacheOpts := []ttlcache.Option[K, V]{
		ttlcache.WithCapacity[K, V](capacity),
	}

	if period > 0 {
		cacheOpts = append(cacheOpts, ttlcache.WithTTL[K, V](period))
	}

	return &disarmerCache[K, V]{
		Cache:    ttlcache.New[K, V](cacheOpts...),
		capacity: capacity,
	}
}

func (c *disarmerCache[K, V]) flush() int {
	c.DeleteExpired()
	return c.Len()
}

func newKillDisarmer(cfg *config.RuntimeSecurityConfig, ruleID rules.RuleID) *killDisarmer {
	kd := &killDisarmer{
		disarmed:      false,
		ruleID:        ruleID,
		disarmedCount: make(map[disarmerType]int64),
	}

	if cfg.EnforcementDisarmerContainerEnabled {
		kd.containerCache = newDisarmerCache[string, bool](uint64(cfg.EnforcementDisarmerContainerMaxAllowed), cfg.EnforcementDisarmerContainerPeriod)
	}

	if cfg.EnforcementDisarmerExecutableEnabled {
		kd.executableCache = newDisarmerCache[string, bool](uint64(cfg.EnforcementDisarmerExecutableMaxAllowed), cfg.EnforcementDisarmerExecutablePeriod)
	}

	return kd
}

func (kd *killDisarmer) allow(cache *disarmerCache[string, bool], typ disarmerType, key string, onDisarm func()) bool {
	kd.Lock()
	defer kd.Unlock()

	if cache == nil {
		return true
	}

	cache.DeleteExpired()
	// if the key is not in the cache, check if the new key causes the number of keys to exceed the capacity
	// otherwise, the key is already in the cache and cache.Get will update its TTL
	if cache.Get(key) == nil {
		alreadyAtCapacity := uint64(cache.Len()) >= cache.capacity
		cache.Set(key, true, ttlcache.DefaultTTL)
		if alreadyAtCapacity && !kd.disarmed {
			kd.disarmed = true
			kd.disarmedCount[typ]++
			onDisarm()
		}
	}

	return !kd.disarmed
}
