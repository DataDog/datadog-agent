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
	killQueueTicker             = time.Millisecond * 250
	killActionFlushDelay        = 2 * time.Second
	killActionQueuedFlushDelay  = killActionDisarmerMaxPeriod + killActionFlushDelay
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

// lock perRuleStatsLock should be acquire first
func (p *ProcessKiller) getRuleStats(ruleID string) *processKillerStats {
	var stats *processKillerStats
	if stats = p.perRuleStats[ruleID]; stats == nil {
		stats = &processKillerStats{}
		p.perRuleStats[ruleID] = stats
	}
	return stats
}

func updateKillActionReport(now time.Time, killQueue *[]killContext, report *KillActionReport, failedToBeKilled []uint32, nbOfKilled int64) {
	report.Lock()
	defer report.Unlock()
	if report.Status == KillActionStatusQueued {
		if slices.Contains(failedToBeKilled, report.Pid) {
			if nbOfKilled > 0 {
				report.Status = KillActionStatusPartiallyPerformed
				report.KilledAt = now
				return
			}
			report.Status = KillActionStatusError
			return

		} else if slices.ContainsFunc(*killQueue, func(kc killContext) bool {
			return kc.pid == int(report.Pid)
		}) {
			report.Status = KillActionStatusPerformed
			report.KilledAt = now
			return
		}
	}
}
func (p *ProcessKiller) updatePendingReportKillPerformed(now time.Time, killQueue *[]killContext, failedToBeKilled []uint32, nbOfKilled int64) {
	p.Lock()
	defer p.Unlock()
	for _, report := range p.pendingReports {
		updateKillActionReport(now, killQueue, report, failedToBeKilled, nbOfKilled)
	}
}

// KillQueuedPidsAndSetNextAlarm performs all pending kills and sets the next alarm if any
func (p *ProcessKiller) KillQueuedPidsAndSetNextAlarm() {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return
	}

	p.ruleDisarmersLock.Lock()
	defer p.ruleDisarmersLock.Unlock()

	now := time.Now()
	var nextAlarm *time.Time
	for _, ruleDisarmer := range p.ruleDisarmers {
		nextAlarm = p.killQueuedPidsAndGetNextAlarm(ruleDisarmer, now, nextAlarm)
	}
	if nextAlarm != nil {
		p.setKillQueueAlarm(nextAlarm)
	} else {
		p.disableKillQueueAlarm()
	}
}

func (p *ProcessKiller) killQueuedPidsAndGetNextAlarm(disarmer *ruleDisarmer, now time.Time, nextAlarm *time.Time) *time.Time {
	disarmer.m.Lock()
	defer disarmer.m.Unlock()

	if disarmer.disarmed || len(disarmer.killQueue) == 0 {
		return nextAlarm
	}

	if now.After(disarmer.killQueueAlarm) {
		failedToBeKilled, nbOfKilled := p.KillProcesses(false, disarmer.ruleID, disarmer.killSignal, disarmer.killQueue)
		p.updatePendingReportKillPerformed(now, &disarmer.killQueue, failedToBeKilled, nbOfKilled)
	} else {
		if nextAlarm == nil || disarmer.killQueueAlarm.Before(*nextAlarm) {
			return &disarmer.killQueueAlarm
		}
	}

	return nextAlarm
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

		now := time.Now()

		// for kills that are performed, we wait for 2sec until we send the event with report
		if report.Status == KillActionStatusPerformed && now.After(report.KilledAt.Add(killActionFlushDelay)) {
			report.resolved = true
			return true
		}

		// for kills that were enqueued, we wait for 1 min (the max default period of any disarmer) + 2sec before sending the event with report
		if report.Status == KillActionStatusQueued && now.After(report.DetectedAt.Add(killActionQueuedFlushDelay)) {
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

// KillAndReport kill and report, returns true if we did try to kill and the report
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

	scope := "process"
	switch kill.Scope {
	case "container", "process", "cgroup":
		scope = kill.Scope
	}

	// if a rule with a kill container scope is triggered outside a container,
	// don't kill anything
	if ev.ProcessContext.Process.ContainerContext.IsNull() && scope == "container" {
		return false, nil
	}
	containerID := ev.ProcessContext.Process.ContainerContext.ContainerID

	var disarmer *ruleDisarmer
	if p.useDisarmers.Load() {
		p.ruleDisarmersLock.Lock()
		if disarmer = p.ruleDisarmers[rule.ID]; disarmer == nil {
			containerParams, executableParams := p.getDisarmerParams(kill)
			disarmer = newRuleDisarmer(rule.ID, containerParams, executableParams)
			p.ruleDisarmers[rule.ID] = disarmer
		}
		p.ruleDisarmersLock.Unlock()

		onActionBlockedByDisarmer := func(dt disarmerType, dismantled bool) *KillActionReport {
			report := &KillActionReport{
				Scope:        scope,
				Signal:       kill.Signal,
				DisarmerType: string(dt),
				CreatedAt:    ev.ProcessContext.ExecTime,
				DetectedAt:   ev.ResolveEventTime(),
				Pid:          ev.ProcessContext.Pid,
				rule:         rule,
			}
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

		if disarmer.container.enabled {
			if containerID != "" {
				disarmer.m.Lock()
				allow, newlyDisarmed := disarmer.allow(disarmer.containerCache, string(containerID))
				if newlyDisarmed {
					if disarmer.dismantled {
						disarmer.dismantledCount[containerDisarmerType]++
						seclog.Warnf("dismantling kill action of rule `%s` because more than %d different containers triggered it in the last %s", rule.ID, disarmer.container.capacity, disarmer.container.period)
					} else {
						disarmer.disarmedCount[containerDisarmerType]++
						seclog.Warnf("disarming kill action of rule `%s` because more than %d different containers triggered it in the last %s", rule.ID, disarmer.container.capacity, disarmer.container.period)
					}
					// update stats
					if len(disarmer.killQueue) > 0 {
						p.perRuleStatsLock.Lock()
						stats := p.getRuleStats(disarmer.ruleID)
						stats.killQueuedDiscardedByDisarm += int64(len(disarmer.killQueue))
						p.perRuleStatsLock.Unlock()
						// clear kill queue list map if not empty
						disarmer.killQueue = nil
					}
				}
				disarmer.m.Unlock()
				if newlyDisarmed {
					p.KillQueuedPidsAndSetNextAlarm()
				}
				if !allow {
					report := onActionBlockedByDisarmer(containerDisarmerType, disarmer.dismantled)
					return false, report
				}
			}
		}

		if disarmer.executable.enabled {
			executable := entry.Process.FileEvent.PathnameStr
			disarmer.m.Lock()
			allow, newlyDisarmed := disarmer.allow(disarmer.executableCache, executable)
			if newlyDisarmed {
				if disarmer.dismantled {
					disarmer.dismantledCount[executableDisarmerType]++
					seclog.Warnf("dismantled kill action of rule `%s` because more than %d different executables triggered it in the last %s", rule.ID, disarmer.executable.capacity, disarmer.executable.period)
				} else {
					disarmer.disarmedCount[executableDisarmerType]++
					seclog.Warnf("disarmed kill action of rule `%s` because more than %d different executables triggered it in the last %s", rule.ID, disarmer.executable.capacity, disarmer.executable.period)
				}
				// update stats
				if len(disarmer.killQueue) > 0 {
					p.perRuleStatsLock.Lock()
					stats := p.getRuleStats(disarmer.ruleID)
					stats.killQueuedDiscardedByDisarm += int64(len(disarmer.killQueue))
					p.perRuleStatsLock.Unlock()
					// clear kill queue list map if not empty
					disarmer.killQueue = nil
				}
			}
			disarmer.m.Unlock()
			if newlyDisarmed {
				p.KillQueuedPidsAndSetNextAlarm()
			}
			if !allow {
				report := onActionBlockedByDisarmer(containerDisarmerType, disarmer.dismantled)
				return false, report
			}
		}
	}

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

	report := &KillActionReport{
		Scope:      scope,
		Signal:     kill.Signal,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		Pid:        ev.ProcessContext.Pid,
		rule:       rule,
	}

	if disarmer != nil && p.warmupEnqueued(disarmer, sig, pcs) {
		log.Warnf("rule %s triggered on first period, putting pids to kill on wait list", rule.ID)
		report.Status = KillActionStatusQueued
		ev.ActionReports = append(ev.ActionReports, report)
		p.Lock()
		p.pendingReports = append(p.pendingReports, report)
		p.Unlock()
		p.perRuleStatsLock.Lock()
		stats := p.getRuleStats(rule.ID)
		stats.killQueued++
		p.perRuleStatsLock.Unlock()
		return false, report
	}

	now := time.Now() // get the current time now to make sure it precedes the any process exit time
	failedPids, nbOfKilled := p.KillProcesses(true, rule.ID, sig, pcs)
	if len(failedPids) == 0 {
		report.KilledAt = now
		report.Status = KillActionStatusPerformed
	} else {
		if nbOfKilled > 0 {
			report.KilledAt = now
			report.Status = KillActionStatusPartiallyPerformed
			log.Warn("some processes failed to be killed in the container with PIDs : ", failedPids)
		} else {
			report.Status = KillActionStatusError
		}
	}

	ev.ActionReports = append(ev.ActionReports, report)
	p.Lock()
	p.pendingReports = append(p.pendingReports, report)
	p.Unlock()
	return true, report
}

// KillProcesses kills the given list of processes, returns the list of pids that failed to be killed (nil if everything went well)
func (p *ProcessKiller) KillProcesses(killDirectly bool, ruleID string, sig int, kcs []killContext) ([]uint32, int64) {
	var failedToKillPids []uint32
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return failedToKillPids, 0
	}
	var processesKilled int64
	for _, pc := range kcs {
		log.Debugf("requesting signal %d to be sent to %d", sig, pc.pid)

		if err := p.os.Kill(uint32(sig), &pc); err != nil {
			seclog.Debugf("failed to kill process %d: %s.", pc.pid, err)
			failedToKillPids = append(failedToKillPids, uint32(pc.pid))

		} else {
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

	return failedToKillPids, processesKilled
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

// Start starts the go rountine responsible for flushing the disarmer caches
func (p *ProcessKiller) Start(ctx context.Context, wg *sync.WaitGroup) {
	if !p.cfg.RuntimeSecurity.EnforcementEnabled {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(disarmerCacheFlushInterval)
		killQueue := time.NewTicker(killQueueTicker)

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
				case <-killQueue.C:
					killQueueAlarm := p.getKillQueueAlarm()
					if killQueueAlarm != nil && time.Now().After(*killQueueAlarm) {
						p.KillQueuedPidsAndSetNextAlarm()
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

// warmupEnqueued returns true if called on the first rule period (and also queue related kills on the quee)
func (p *ProcessKiller) warmupEnqueued(rd *ruleDisarmer, signal int, kcs []killContext) bool {
	if time.Now().After(rd.warmupEnd) {
		return false
	}

	rd.killSignal = signal // should not change
	if len(rd.killQueue) == 0 {
		rd.killQueue = kcs
	} else {
		rd.killQueue = append(rd.killQueue, kcs...)
		// sort and compact to ensure we don't duplicate kill actions
		slices.SortFunc(rd.killQueue, func(a, b killContext) int {
			if a.pid < b.pid {
				return -1
			}
			return 1
		})
		rd.killQueue = slices.CompactFunc(rd.killQueue, func(a, b killContext) bool {
			return a.pid == b.pid
		})
	}
	rd.killQueueAlarm = rd.warmupEnd
	p.setKillQueueAlarm(&rd.killQueueAlarm)
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

	killQueue      []killContext
	killSignal     int
	killQueueAlarm time.Time

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
