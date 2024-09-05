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

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultKillActionFlushDelay = 2 * time.Second
	dismarmerCacheFlushInterval = 5 * time.Second
)

// ProcessKiller defines a process killer structure
type ProcessKiller struct {
	sync.Mutex

	cfg *config.Config

	pendingReports   []*KillActionReport
	binariesExcluded []*eval.Glob
	sourceAllowed    []string

	ruleDisarmersLock sync.Mutex
	ruleDisarmers     map[rules.RuleID]*killDisarmer
}

// NewProcessKiller returns a new ProcessKiller
func NewProcessKiller(cfg *config.Config) (*ProcessKiller, error) {
	p := &ProcessKiller{
		cfg:           cfg,
		ruleDisarmers: make(map[rules.RuleID]*killDisarmer),
		sourceAllowed: cfg.RuntimeSecurity.EnforcementRuleSourceAllowed,
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
		var dismarmer *killDisarmer
		p.ruleDisarmersLock.Lock()
		if dismarmer = p.ruleDisarmers[rule.ID]; dismarmer == nil {
			dismarmer = newKillDisarmer(rsConfig, rule.ID)
			p.ruleDisarmers[rule.ID] = dismarmer
		}
		p.ruleDisarmersLock.Unlock()

		if rsConfig.EnforcementDisarmerContainerEnabled {
			if containerID := ev.FieldHandlers.ResolveContainerID(ev, ev.ContainerContext); containerID != "" {
				if !dismarmer.allow(dismarmer.containerCache, containerID, func() {
					seclog.Warnf("disarming kill action of rule `%s` because more than %d different containers triggered it in the last %s", rule.ID, dismarmer.containerCache.capacity, rsConfig.EnforcementDisarmerContainerPeriod)
				}) {
					seclog.Warnf("skipping kill action of rule `%s` because it has been disarmed", rule.ID)
					return
				}
			}
		}

		if rsConfig.EnforcementDisarmerExecutableEnabled {
			executable := entry.Process.FileEvent.PathnameStr
			if !dismarmer.allow(dismarmer.executableCache, executable, func() {
				seclog.Warnf("disarmed kill action of rule `%s` because more than %d different executables triggered it in the last %s", rule.ID, dismarmer.executableCache.capacity, rsConfig.EnforcementDisarmerExecutablePeriod)
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

	killedAt := time.Now()
	for _, pid := range pids {
		log.Debugf("requesting signal %s to be sent to %d", signal, pid)

		if err := killFnc(uint32(pid), uint32(sig)); err != nil {
			seclog.Debugf("failed to kill process %d: %s", pid, err)
		}
	}

	p.Lock()
	defer p.Unlock()

	report := &KillActionReport{
		Scope:      scope,
		Signal:     signal,
		Pid:        ev.ProcessContext.Pid,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		KilledAt:   killedAt,
		Rule:       rule,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)
}

// Reset resets the disarmer state
func (p *ProcessKiller) Reset() {
	p.ruleDisarmersLock.Lock()
	clear(p.ruleDisarmers)
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
		ticker := time.NewTicker(dismarmerCacheFlushInterval)
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

type killDisarmer struct {
	sync.Mutex
	disarmed        bool
	ruleID          rules.RuleID
	containerCache  *disarmerCache[string, bool]
	executableCache *disarmerCache[string, bool]
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
		disarmed: false,
		ruleID:   ruleID,
	}

	if cfg.EnforcementDisarmerContainerEnabled {
		kd.containerCache = newDisarmerCache[string, bool](uint64(cfg.EnforcementDisarmerContainerMaxAllowed), cfg.EnforcementDisarmerContainerPeriod)
	}

	if cfg.EnforcementDisarmerExecutableEnabled {
		kd.executableCache = newDisarmerCache[string, bool](uint64(cfg.EnforcementDisarmerExecutableMaxAllowed), cfg.EnforcementDisarmerExecutablePeriod)
	}

	return kd
}

func (kd *killDisarmer) allow(cache *disarmerCache[string, bool], key string, onDisarm func()) bool {
	kd.Lock()
	defer kd.Unlock()

	if kd.disarmed {
		return false
	}

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
			onDisarm()
		}
	}

	return !kd.disarmed
}
