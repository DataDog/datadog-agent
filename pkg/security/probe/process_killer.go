// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
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

// ProcessKillerOS interface defines an os specific process killer
type ProcessKillerOS interface {
	Kill(sig uint32, pc *killContext) error
	getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error)
}

const (
	disarmerCacheFlushInterval = 5 * time.Second
	// killActionDisarmerMaxPeriod represents the maximum disarmer period
	killActionDisarmerMaxPeriod = time.Second * 60
	pendingKillsTicker          = time.Millisecond * 250
	killActionFlushDelay        = 2 * time.Second
	pendingKillsFlushDelay      = killActionDisarmerMaxPeriod + killActionFlushDelay
)

// ProcessKiller defines a process killer structure
type ProcessKiller struct {
	sync.Mutex

	cfg *config.Config
	os  ProcessKillerOS

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

	currentKillQueueAlarmLock sync.Mutex
	currentKillQueueAlarm     *time.Time
}

type processKillerStats struct {
	processesKilledDirectly     int64
	processesKilledAfterQueue   int64
	killQueued                  int64
	killQueuedDiscardedByDisarm int64
}

// NewProcessKiller returns a new ProcessKiller
func NewProcessKiller(cfg *config.Config, pkos ProcessKillerOS) (*ProcessKiller, error) {
	if pkos == nil {
		pkos = NewProcessKillerOS(nil, nil)
	}
	p := &ProcessKiller{
		cfg:             cfg,
		os:              pkos,
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

func resolveKillScope(kill *rules.KillDefinition) string {
	switch kill.Scope {
	case "container", "process", "cgroup":
		return kill.Scope
	}
	return "process"
}

func newKillActionReport(scope string, kill *rules.KillDefinition, ev *model.Event, rule *rules.Rule) *KillActionReport {
	report := &KillActionReport{
		Scope:      scope,
		Signal:     kill.Signal,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		Pid:        ev.ProcessContext.Pid,
		rule:       rule,
	}
	if !ev.ProcessContext.Process.ContainerContext.IsNull() {
		report.containerContext.ID = string(ev.ProcessContext.Process.ContainerContext.ContainerID)
		report.containerContext.CreatedAt = ev.ProcessContext.Process.ContainerContext.CreatedAt
	}
	return report
}

func (p *ProcessKiller) registerReport(report *KillActionReport, ev *model.Event) {
	ev.ActionReports = append(ev.ActionReports, report)
	p.Lock()
	p.pendingReports = append(p.pendingReports, report)
	p.Unlock()
}

// checkDisarmerCache checks a single disarmer cache (container or executable) and handles
// disarm/dismantle transitions. Returns (kill action allowed to proceed, dismantled flag).
func (p *ProcessKiller) checkDisarmerCache(disarmer *ruleDisarmer, dt disarmerType, key string, ruleID string) (bool, bool) {
	var cache *disarmerCache[string, bool]
	var params *disarmerParams
	switch dt {
	case containerDisarmerType:
		cache = disarmer.containerCache
		params = &disarmer.container
	case executableDisarmerType:
		cache = disarmer.executableCache
		params = &disarmer.executable
	}

	disarmer.m.Lock()
	allow, newlyDisarmed := disarmer.allow(cache, key)
	if newlyDisarmed {
		if disarmer.dismantled {
			disarmer.dismantledCount[dt]++
			seclog.Warnf("dismantling kill action of rule `%s` because more than %d different %ss triggered it in the last %s", ruleID, params.capacity, dt, params.period)
		} else {
			disarmer.disarmedCount[dt]++
			seclog.Warnf("disarming kill action of rule `%s` because more than %d different %ss triggered it in the last %s", ruleID, params.capacity, dt, params.period)
		}
		if len(disarmer.pendingReports) > 0 {
			var totalQueued int64
			for _, r := range disarmer.pendingReports {
				r.Lock()
				totalQueued += int64(len(r.pendingKills))
				r.Unlock()
			}
			p.perRuleStatsLock.Lock()
			stats := p.getRuleStats(disarmer.ruleID)
			stats.killQueuedDiscardedByDisarm += totalQueued
			p.perRuleStatsLock.Unlock()
			disarmer.pendingReports = nil
		}
	}
	// get it before unlocking the mutex
	dismantled := disarmer.dismantled
	disarmer.m.Unlock()

	if newlyDisarmed {
		p.processPendingKills()
		p.updateNextAlarm()
	}
	return allow, dismantled
}

func (p *ProcessKiller) reportBlockedByDisarmer(scope string, kill *rules.KillDefinition, ev *model.Event, rule *rules.Rule, dt disarmerType, dismantled bool) *KillActionReport {
	report := newKillActionReport(scope, kill, ev, rule)
	report.DisarmerType = string(dt)
	if dismantled {
		report.Status = KillActionStatusRuleDismantled
		seclog.Warnf("skipping kill action of rule `%s` because it has been dismantled", rule.ID)
	} else {
		report.Status = KillActionStatusRuleDisarmed
		seclog.Warnf("skipping kill action of rule `%s` because it has been disarmed", rule.ID)
	}
	ev.ActionReports = append(ev.ActionReports, report)
	return report
}

// KillAndReport attempts to kill processes matching the rule's kill definition and returns
// whether the kill was performed along with the action report.
func (p *ProcessKiller) KillAndReport(kill *rules.KillDefinition, rule *rules.Rule, ev *model.Event) (bool, *KillActionReport) {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return false, nil
	}

	if !p.isRuleAllowed(rule) {
		log.Warnf("unable to kill, the source is not allowed: %v", rule)
		return false, nil
	}

	entry, exists := ev.ResolveProcessCacheEntry(nil)
	if !exists {
		return false, nil
	}

	scope := resolveKillScope(kill)

	// if a rule with a kill container scope is triggered outside a container,
	// don't kill anything
	if ev.ProcessContext.Process.ContainerContext.IsNull() && scope == "container" {
		return false, nil
	}
	// check if the rule has a disarmer, if not, create one
	var disarmer *ruleDisarmer
	if p.useDisarmers.Load() {
		p.ruleDisarmersLock.Lock()
		if disarmer = p.ruleDisarmers[rule.ID]; disarmer == nil {
			containerParams, executableParams := p.getDisarmerParams(kill)
			disarmer = newRuleDisarmer(rule.ID, containerParams, executableParams)
			p.ruleDisarmers[rule.ID] = disarmer
		}
		p.ruleDisarmersLock.Unlock()

		if disarmer.container.enabled {
			containerID := string(ev.ProcessContext.Process.ContainerContext.ContainerID)
			if containerID != "" {
				if allow, dismantled := p.checkDisarmerCache(disarmer, containerDisarmerType, containerID, rule.ID); !allow {
					return false, p.reportBlockedByDisarmer(scope, kill, ev, rule, containerDisarmerType, dismantled)
				}
			}
		}

		if disarmer.executable.enabled {
			executable := entry.Process.FileEvent.PathnameStr
			if allow, dismantled := p.checkDisarmerCache(disarmer, executableDisarmerType, executable, rule.ID); !allow {
				return false, p.reportBlockedByDisarmer(scope, kill, ev, rule, executableDisarmerType, dismantled)
			}
		}
	}
	// get the list of pids to kill
	// if the scope is process, the list will contain only one pid
	pcs, err := p.os.getProcesses(scope, ev, entry)
	if err != nil {
		log.Errorf("unable to kill: %s", err)
		return false, nil
	}

	// if one pids is not allowed don't kill anything
	if killAllowed, err := p.isKillAllowed(pcs); !killAllowed {
		log.Warnf("unable to kill: %v", err)
		return false, nil
	}

	sig := model.SignalConstants[kill.Signal]
	report := newKillActionReport(scope, kill, ev, rule)

	// if the rule is triggered for the first time and a disarmer is active, put the pids to kill on the wait list and don't kill them now
	if isWarmupPeriod(disarmer) {
		p.enqueueDuringWarmup(disarmer, sig, report, pcs)
		log.Warnf("rule %s triggered on first period, putting pids to kill on wait list", rule.ID)
		report.Status = KillActionStatusQueued
		p.registerReport(report, ev)
		// update stats
		p.perRuleStatsLock.Lock()
		stats := p.getRuleStats(rule.ID)
		stats.killQueued++
		p.perRuleStatsLock.Unlock()
		return false, report
	}
	// kill the processes
	killedAt := time.Now() // get the current time now to make sure it precedes any process exit time
	// we need to keep to lock when we kill and update
	report.Lock()
	// populate pendingKills so updateKillActionReport can map failed/killed pids back
	// to this report (same invariant as the disarmer warmup path).
	report.pendingKills = pcs
	failedPids, killedPids := p.KillProcesses(true, rule.ID, sig, pcs)
	updateKillActionReport(report, killedAt, failedPids, killedPids)
	report.Unlock()
	if len(failedPids) > 0 && len(killedPids) > 0 {
		log.Warn("some processes failed to be killed with PIDs: ", failedPids)
	}
	p.registerReport(report, ev)
	return true, report
}

// KillProcesses kills the given list of processes, returns the list of pids that failed to be killed (nil if everything went well)
func (p *ProcessKiller) KillProcesses(killDirectly bool, ruleID string, sig int, kcs []killContext) ([]uint32, []uint32) {
	var failedPids []uint32
	var killedPids []uint32
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return failedPids, killedPids
	}
	var processesKilled int64
	for _, pc := range kcs {
		log.Debugf("requesting signal %d to be sent to %d", sig, pc.pid)

		if err := p.os.Kill(uint32(sig), &pc); err != nil {
			seclog.Debugf("failed to kill process %d: %s.", pc.pid, err)
			failedPids = append(failedPids, uint32(pc.pid))

		} else {
			killedPids = append(killedPids, uint32(pc.pid))
			processesKilled++
		}
	}

	p.perRuleStatsLock.Lock()
	stats := p.getRuleStats(ruleID)
	if killDirectly {
		stats.processesKilledDirectly += processesKilled
	} else {
		stats.processesKilledAfterQueue += processesKilled
	}
	p.perRuleStatsLock.Unlock()

	return failedPids, killedPids
}

// Start starts the go routine responsible for flushing the disarmer caches and the pending kill queue
func (p *ProcessKiller) Start(ctx context.Context, wg *sync.WaitGroup) {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(disarmerCacheFlushInterval)
		pendingKillsTick := time.NewTicker(pendingKillsTicker)

		defer ticker.Stop()
		defer pendingKillsTick.Stop()
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
				case <-pendingKillsTick.C:
					alarm := p.getKillQueueAlarm()
					if alarm != nil && time.Now().After(*alarm) {
						p.processPendingKills()
						p.updateNextAlarm()
					}
					break

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
						disarmer.m.Lock()
						if !disarmer.dismantled {
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
						}
						disarmer.m.Unlock()
					}
					p.ruleDisarmersLock.Unlock()
				}
			}
		}
	}()
}

// updateNextAlarm computes and sets the next pending kills alarm across all disarmers.
func (p *ProcessKiller) updateNextAlarm() {
	p.ruleDisarmersLock.Lock()
	defer p.ruleDisarmersLock.Unlock()

	var nextAlarm *time.Time
	for _, disarmer := range p.ruleDisarmers {
		disarmer.m.Lock()
		if !disarmer.disarmed && len(disarmer.pendingReports) > 0 {
			if nextAlarm == nil || disarmer.pendingKillsAlarm.Before(*nextAlarm) {
				t := disarmer.pendingKillsAlarm
				nextAlarm = &t
			}
		}
		disarmer.m.Unlock()
	}
	if nextAlarm != nil {
		p.setKillQueueAlarm(nextAlarm)
	} else {
		p.disableKillQueueAlarm()
	}
}

// processPendingKills iterates over all disarmers and executes pending kills whose alarm has elapsed.
func (p *ProcessKiller) processPendingKills() {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return
	}
	p.ruleDisarmersLock.Lock()
	defer p.ruleDisarmersLock.Unlock()

	now := time.Now()
	for _, disarmer := range p.ruleDisarmers {
		p.killPendingForDisarmer(disarmer, now)
	}
}

// killPendingForDisarmer executes pending kills for a single disarmer.
func (p *ProcessKiller) killPendingForDisarmer(disarmer *ruleDisarmer, now time.Time) {
	disarmer.m.Lock()
	defer disarmer.m.Unlock()

	if disarmer.disarmed || len(disarmer.pendingReports) == 0 || !now.After(disarmer.pendingKillsAlarm) {
		return
	}

	// Hold the locks until the end of the function to avoid a conflict with HandleProcessExit and FlushPendingReports
	for _, r := range disarmer.pendingReports {
		r.Lock()
		defer r.Unlock()
	}

	// Drop reports that were already resolved (e.g. aborted because all PIDs
	// exited before the warmup period elapsed). Without this, updateKillActionReport
	// would overwrite their final status.
	disarmer.pendingReports = slices.DeleteFunc(disarmer.pendingReports, func(r *KillActionReport) bool {
		return r.resolved
	})
	// After dropping the reports, check if there are any left
	if len(disarmer.pendingReports) == 0 {
		return
	}

	var allKills []killContext
	for _, r := range disarmer.pendingReports {
		allKills = append(allKills, r.pendingKills...)
	}
	slices.SortFunc(allKills, func(a, b killContext) int {
		if a.pid < b.pid {
			return -1
		}
		return 1
	})
	allKills = slices.CompactFunc(allKills, func(a, b killContext) bool {
		return a.pid == b.pid
	})

	if len(allKills) == 0 {
		seclog.Debugf("no pending kill for rule `%s`", disarmer.ruleID)
	}
	failedPids, killedPids := p.KillProcesses(false, disarmer.ruleID, disarmer.killSignal, allKills)
	for _, r := range disarmer.pendingReports {
		updateKillActionReport(r, now, failedPids, killedPids)
	}
	disarmer.pendingReports = nil
}

// updateKillActionReport updates the report status based on the outcome of KillProcesses.
func updateKillActionReport(report *KillActionReport, now time.Time, failedPids []uint32, killedPids []uint32) {
	// failedPids and killedPids aggregate one batch over several reports (disarmer warmup path).
	// so we need to check which pid was killed or failed for each report
	var failedCount, killedCount int
	if len(report.pendingKills) > 0 {
		reportFailedPids := make([]uint32, 0, len(report.pendingKills))
		reportKilledPids := make([]uint32, 0, len(report.pendingKills))
		// For each pending kill, check if it was killed or failed
		for _, kc := range report.pendingKills {
			pid := uint32(kc.pid)
			switch {
			case slices.Contains(failedPids, pid):
				reportFailedPids = append(reportFailedPids, pid)
			case slices.Contains(killedPids, pid):
				reportKilledPids = append(reportKilledPids, pid)
			default:
				// should never happen
				seclog.Infof("updateKillActionReport called with unknown pid in the pendingKills list %d", pid)
			}
		}
		failedCount = len(reportFailedPids)
		killedCount = len(reportKilledPids)
	} else {
		// should never happen
		seclog.Infof("updateKillActionReport called with empty pendingKills")
		return
	}

	if failedCount == 0 {
		report.Status = KillActionStatusPerformed
		report.KilledAt = now
	} else if killedCount > 0 {
		// Partially performed can happen if a process exited before it was killed
		// This mostly happens without any disarmer since we never remove any process from the list before killing
		report.Status = KillActionStatusPartiallyPerformed
		report.KilledAt = now
	} else {
		report.Status = KillActionStatusError
	}
}

// FlushPendingReports flush pending reports
func (p *ProcessKiller) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()

		now := time.Now()

		// for kills that are performed, we wait for 2sec until we send the event with report
		if report.Status == KillActionStatusPerformed && now.After(report.KilledAt.Add(killActionFlushDelay)) {
			report.resolved = true
			return true
		}

		// for kills that were enqueued, we wait for 1 min (the max default period of any disarmer) + 2sec before sending the event with report
		if report.Status == KillActionStatusQueued && now.After(report.DetectedAt.Add(pendingKillsFlushDelay)) {
			report.resolved = true
			return true
		}

		return false
	})
}

// HandleProcessExited handles process exited events.
// For queued reports, it removes the exited PID from pendingKills.
// If all pending kills are gone, the report is resolved as kill_aborted.
func (p *ProcessKiller) HandleProcessExited(event *model.Event) {
	p.Lock()
	defer p.Unlock()

	exitedPid := event.ProcessContext.Pid

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()
		// update the exitedAt field if the exited PID is the same as the one in the report
		if report.Pid == exitedPid {
			report.ExitedAt = event.ProcessContext.ExitTime
		}
		switch report.Status {
		// if the report is queued or error, remove the exited PID from pendingKills
		// queued: this will handle the case where a process exited before we tried to kill
		// error: this will handle the case where we failed to kill a process before it exited (race condition or another rule killed it before)
		case KillActionStatusQueued, KillActionStatusError:
			report.pendingKills = slices.DeleteFunc(report.pendingKills, func(kc killContext) bool {
				return uint32(kc.pid) == exitedPid
			})
			if len(report.pendingKills) == 0 {
				report.Status = KillActionStatusKillAborted
				// Update the exitedAt field since the kill was aborted and won't be performed any more
				report.ExitedAt = event.ProcessContext.ExitTime
				report.resolved = true
				return true
			}
		// if the report is performed or partially performed, it is resolved (no need to wait the flush delay)
		case KillActionStatusPartiallyPerformed, KillActionStatusPerformed:
			if report.Pid == exitedPid {
				report.resolved = true
				return true
			}
		}
		return false
	})
}

func (p *ProcessKiller) isKillAllowed(kcs []killContext) (bool, error) {
	p.Lock()
	if !p.enabled {
		p.Unlock()
		return false, errors.New("the enforcement capability is disabled")
	}
	p.Unlock()

	for _, pc := range kcs {
		if pc.pid <= 1 || uint32(pc.pid) == utils.Getpid() {
			return false, fmt.Errorf("process with pid %d cannot be killed", pc.pid)
		}

		if slices.ContainsFunc(p.binariesExcluded, func(glob *eval.Glob) bool {
			return glob.Matches(pc.path)
		}) {
			return false, fmt.Errorf("process `%s`(%d) is protected", pc.path, pc.pid)
		}
	}
	return true, nil
}

func (p *ProcessKiller) isRuleAllowed(rule *rules.Rule) bool {
	return slices.Contains(p.sourceAllowed, rule.Policy.Source)
}

func (p *ProcessKiller) getKillQueueAlarm() *time.Time {
	p.currentKillQueueAlarmLock.Lock()
	defer p.currentKillQueueAlarmLock.Unlock()

	return p.currentKillQueueAlarm
}

func (p *ProcessKiller) disableKillQueueAlarm() {
	p.currentKillQueueAlarmLock.Lock()
	defer p.currentKillQueueAlarmLock.Unlock()

	p.currentKillQueueAlarm = nil
}

func (p *ProcessKiller) setKillQueueAlarm(alarm *time.Time) {
	p.currentKillQueueAlarmLock.Lock()
	defer p.currentKillQueueAlarmLock.Unlock()

	if p.currentKillQueueAlarm == nil || alarm.Before(*p.currentKillQueueAlarm) {
		p.currentKillQueueAlarm = alarm
		return
	}
}

func isWarmupPeriod(rd *ruleDisarmer) bool {
	return rd != nil && time.Now().Before(rd.warmupEnd)
}

// enqueueDuringWarmup enqueues the kill contexts into the report and adds it to the disarmer's
// pending reports if the warmup period has not yet elapsed. Returns true if enqueued.
func (p *ProcessKiller) enqueueDuringWarmup(rd *ruleDisarmer, signal int, report *KillActionReport, kcs []killContext) bool {
	rd.m.Lock()
	defer rd.m.Unlock()
	rd.killSignal = signal
	report.pendingKills = kcs
	rd.pendingReports = append(rd.pendingReports, report)
	rd.pendingKillsAlarm = rd.warmupEnd
	p.setKillQueueAlarm(&rd.pendingKillsAlarm)
	return true
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
	m               sync.Mutex
	ruleID          string
	createdAt       time.Time
	warmupEnd       time.Time
	disarmed        bool
	dismantled      bool
	container       disarmerParams
	containerCache  *disarmerCache[string, bool]
	executable      disarmerParams
	executableCache *disarmerCache[string, bool]

	pendingReports    []*KillActionReport
	killSignal        int
	pendingKillsAlarm time.Time

	// stats
	disarmedCount   map[disarmerType]int64
	dismantledCount map[disarmerType]int64
	rearmedCount    int64
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

func newRuleDisarmer(ruleID string, containerParams *disarmerParams, executableParams *disarmerParams) *ruleDisarmer {
	rd := &ruleDisarmer{
		ruleID:          ruleID,
		createdAt:       time.Now(),
		disarmed:        false,
		container:       *containerParams,
		executable:      *executableParams,
		disarmedCount:   make(map[disarmerType]int64),
		dismantledCount: make(map[disarmerType]int64),
	}

	period := time.Duration(0)

	if rd.container.enabled {
		rd.containerCache = newDisarmerCache[string, bool](containerParams)
		period = max(period, rd.container.period)
		rd.warmupEnd = rd.createdAt.Add(period)
	}

	if rd.executable.enabled {
		rd.executableCache = newDisarmerCache[string, bool](executableParams)
		period = max(period, rd.executable.period)
		rd.warmupEnd = rd.createdAt.Add(period)
	}

	return rd
}

func (p *ProcessKiller) getDisarmerParams(kill *rules.KillDefinition) (*disarmerParams, *disarmerParams) {
	var containerParams, executableParams disarmerParams

	if !kill.DisableContainerDisarmer && p.cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled {
		containerParams.enabled = true
		containerParams.capacity = uint64(p.cfg.RuntimeSecurity.EnforcementDisarmerContainerMaxAllowed)
		containerParams.period = p.cfg.RuntimeSecurity.EnforcementDisarmerContainerPeriod
	}

	if !kill.DisableExecutableDisarmer && p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled {
		executableParams.enabled = true
		executableParams.capacity = uint64(p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableMaxAllowed)
		executableParams.period = p.cfg.RuntimeSecurity.EnforcementDisarmerExecutablePeriod
	}

	// cap the disarmer periods to killActionDisarmerMaxPeriod
	if containerParams.period > killActionDisarmerMaxPeriod {
		seclog.Warnf("Runtime security enforcement container disarmer period configured to %v, which is longer than maximum allowed, capping it at %v", containerParams.period, killActionDisarmerMaxPeriod)
		containerParams.period = killActionDisarmerMaxPeriod
	}
	if executableParams.period > killActionDisarmerMaxPeriod {
		seclog.Warnf("Runtime security enforcement executable disarmer period configured to %v, which is longer than maximum allowed, capping it at %v", executableParams.period, killActionDisarmerMaxPeriod)
		executableParams.period = killActionDisarmerMaxPeriod
	}

	return &containerParams, &executableParams
}

// allow return true if the given key is allowed to be killed, and whether it was newly disarmed
// should be called with the ruleDisarmer's mutex lock held
func (rd *ruleDisarmer) allow(cache *disarmerCache[string, bool], key string) (bool, bool) {
	if rd.dismantled {
		return false, false
	}

	if cache == nil {
		return true, false
	}

	var newlyDisarmed bool
	cache.DeleteExpired()
	// if the key is not in the cache, check if the new key causes the number of keys to exceed the capacity
	// otherwise, the key is already in the cache and cache.Get will update its TTL
	if cache.Get(key) == nil {
		alreadyAtCapacity := uint64(cache.Len()) >= cache.capacity
		cache.Set(key, true, ttlcache.DefaultTTL)
		if alreadyAtCapacity && !rd.disarmed {
			newlyDisarmed = true
			rd.disarmed = true
			if time.Now().Before(rd.warmupEnd) {
				rd.dismantled = true
			}
		}
	}

	return !rd.disarmed, newlyDisarmed
}

// SendStats sends runtime security enforcement statistics to Datadog
func (p *ProcessKiller) SendStats(statsd statsd.ClientInterface) {
	p.perRuleStatsLock.Lock()
	for ruleID, stats := range p.perRuleStats {
		ruleIDTag := "rule_id:" + string(ruleID)

		if stats.processesKilledDirectly > 0 {
			_ = statsd.Count(metrics.MetricEnforcementProcessKilled, stats.processesKilledDirectly, []string{ruleIDTag, "queued:false"}, 1)
			stats.processesKilledDirectly = 0
		}
		if stats.processesKilledAfterQueue > 0 {
			_ = statsd.Count(metrics.MetricEnforcementProcessKilled, stats.processesKilledAfterQueue, []string{ruleIDTag, "queued:true"}, 1)
			stats.processesKilledAfterQueue = 0
		}
		if stats.killQueued > 0 {
			_ = statsd.Count(metrics.MetricEnforcementKillQueued, stats.killQueued, []string{ruleIDTag}, 1)
			stats.processesKilledAfterQueue = 0
		}
		if stats.killQueuedDiscardedByDisarm > 0 {
			_ = statsd.Count(metrics.MetricEnforcementKillQueuedDiscarded, stats.killQueuedDiscardedByDisarm, []string{ruleIDTag}, 1)
			stats.processesKilledAfterQueue = 0
		}
	}
	p.perRuleStatsLock.Unlock()

	p.ruleDisarmersLock.Lock()
	for ruleID, disarmer := range p.ruleDisarmers {
		ruleIDTag := []string{
			"rule_id:" + string(ruleID),
		}

		disarmer.m.Lock()
		for disarmerType, count := range disarmer.disarmedCount {
			if count > 0 {
				tags := append([]string{"disarmer_type:" + string(disarmerType)}, ruleIDTag...)
				_ = statsd.Count(metrics.MetricEnforcementRuleDisarmed, count, tags, 1)
				disarmer.disarmedCount[disarmerType] = 0
			}
		}
		for disarmerType, count := range disarmer.dismantledCount {
			if count > 0 {
				tags := append([]string{"disarmer_type:" + string(disarmerType)}, ruleIDTag...)
				_ = statsd.Count(metrics.MetricEnforcementRuleDismantled, count, tags, 1)
				disarmer.dismantledCount[disarmerType] = 0
			}
		}
		if disarmer.rearmedCount > 0 {
			_ = statsd.Count(metrics.MetricEnforcementRuleRearmed, disarmer.rearmedCount, ruleIDTag, 1)
			disarmer.rearmedCount = 0
		}
		disarmer.m.Unlock()
	}
	p.ruleDisarmersLock.Unlock()
}

// Reset the state and statistics of the process killer
func (p *ProcessKiller) Reset(rs *rules.RuleSet) {
	if p.cfg.RuntimeSecurity.EnforcementEnabled {
		var ruleSetHasKillAction bool

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
				break rules
			}
		}

		configHasKillDisarmer := p.cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled || p.cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled
		if ruleSetHasKillAction && configHasKillDisarmer {
			p.useDisarmers.Store(true)
			p.disarmerStateCh <- running
			p.disableKillQueueAlarm()
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

// lock perRuleStatsLock should be acquire first
func (p *ProcessKiller) getRuleStats(ruleID string) *processKillerStats {
	var stats *processKillerStats
	if stats = p.perRuleStats[ruleID]; stats == nil {
		stats = &processKillerStats{}
		p.perRuleStats[ruleID] = stats
	}
	return stats
}

// SetState sets the state - enabled or disabled - for the process killer
func (p *ProcessKiller) SetState(enabled bool) {
	p.Lock()
	defer p.Unlock()

	p.enabled = enabled
}
