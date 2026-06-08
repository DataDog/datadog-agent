// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type FakeProcessKillerOS struct{}

func (fpk *FakeProcessKillerOS) Kill(_ uint32, _ *killContext) error {
	return nil // fake kill
}

func (fpk *FakeProcessKillerOS) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error) {
	kcs := []killContext{
		{
			pid:  int(ev.ProcessContext.Pid),
			path: ev.ProcessContext.FileEvent.PathnameStr,
		},
	}
	if entry.Process.ContainerContext.ContainerID != "" && scope == "container" {
		kcs = append(kcs, []killContext{
			{
				pid:  int(ev.ProcessContext.Pid + 1),
				path: ev.ProcessContext.FileEvent.PathnameStr + "_1",
			},
			{
				pid:  int(ev.ProcessContext.Pid + 2),
				path: ev.ProcessContext.FileEvent.PathnameStr + "_2",
			},
		}...)
	}
	return kcs, nil
}

func (p *ProcessKiller) vacumChan() {
	<-p.disarmerStateCh
}

func (p *ProcessKiller) getDisarmer(ruleID string) *ruleDisarmer {
	disarmer, ok := p.ruleDisarmers[ruleID]
	if ok && disarmer != nil {
		return disarmer
	}
	return nil
}

func (p *ProcessKiller) getRuleStatsNoAlloc(ruleID string) *processKillerStats {
	stats, ok := p.perRuleStats[ruleID]
	if ok && stats != nil {
		return stats
	}
	return nil
}

func assertKillEvent(t *testing.T, pk *ProcessKiller, rule *rules.Rule, container, executable string, pid uint32, status KillActionStatus, scope string) {
	event := craftFakeEvent(container, executable, pid)
	// First kill should be enqueued
	killed, _ := pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
	if status == KillActionStatusPerformed {
		assert.True(t, killed)
	} else {
		assert.False(t, killed)
	}
	assert.Equal(t, 1, len(event.ActionReports))
	report, ok := event.ActionReports[0].(*KillActionReport)
	assert.True(t, ok)
	assert.Equal(t, "SIGKILL", report.Signal)
	assert.Equal(t, scope, report.Scope)
	assert.Equal(t, status, report.Status)
	assert.Equal(t, pid, report.Pid)
	assert.Equal(t, false, report.resolved)
}

func assertProcessKillEvent(t *testing.T, pk *ProcessKiller, rule *rules.Rule, container, executable string, pid uint32, status KillActionStatus) {
	assertKillEvent(t, pk, rule, container, executable, pid, status, "process")
}

func assertContainerKillEvent(t *testing.T, pk *ProcessKiller, rule *rules.Rule, container, executable string, pid uint32, status KillActionStatus) {
	assertKillEvent(t, pk, rule, container, executable, pid, status, "container")
}

func TestProcessKillerExclusion(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementBinaryExcluded: []string{"excluded"},
		},
	}
	pk, err := NewProcessKiller(cfg, nil)
	assert.NoError(t, err)

	allowed, err := pk.isKillAllowed([]killContext{{path: "excluded", pid: 123}})
	assert.False(t, allowed)
	assert.Error(t, err)

	allowed, err = pk.isKillAllowed([]killContext{{path: "allowed", pid: 123}})
	assert.True(t, allowed)
	assert.NoError(t, err)
}

func craftKillRule(t *testing.T, id, scope string) (*rules.Rule, *rules.RuleSet) {
	rule := &rules.Rule{
		PolicyRule: &rules.PolicyRule{
			Def: &rules.RuleDefinition{
				ID:         id,
				Expression: `exec.file.path == "/tmp/malware"`,
				Actions: []*rules.ActionDefinition{
					{
						Kill: &rules.KillDefinition{
							Signal: "SIGKILL",
							Scope:  scope,
						},
					},
				},
			},
			Policy: rules.PolicyInfo{
				Source: "test",
			},
			Actions: []*rules.Action{
				{
					Def: &rules.ActionDefinition{
						Kill: &rules.KillDefinition{
							Signal: "SIGKILL",
							Scope:  scope,
						},
					},
				},
			},
		},
		Rule: &eval.Rule{
			ID: id,
		},
	}

	opts := rules.NewRuleOpts(map[eval.EventType]bool{"*": true})
	ruleSet := rules.NewRuleSet(&model.Model{}, nil, opts, &eval.Opts{})
	_, err := ruleSet.AddRule(rule.PolicyRule)
	assert.NoError(t, err)

	return rule, ruleSet
}

func craftFakeEvent(containerID, executable string, pid uint32) *model.Event {
	event := model.NewFakeEvent()
	event.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				ContainerContext: model.ContainerContext{
					ContainerID: containerutils.ContainerID(containerID),
				},
				FileEvent: model.FileEvent{
					PathnameStr: executable,
				},
				PIDContext: model.PIDContext{
					Pid: pid,
				},
			},
		},
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext
	event.Type = uint32(model.ExitEventType) // don't matter
	event.Exit = model.ExitEvent{
		Process: &model.Process{
			FileEvent: model.FileEvent{
				PathnameStr: executable,
			},
			PIDContext: model.PIDContext{
				Pid: pid,
			},
		},
	}
	return event
}

func TestProcessKillerDisarmers(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  1,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 1,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "process")

	t.Run("dismantle-rule-kill-action-by-executable", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 456, KillActionStatusQueued)

		// Third kill should dismantle the rule
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 789, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("dismantle-rule-kill-action-by-container", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 456, KillActionStatusQueued)

		// Third kill should dismantle the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 789, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("disarm-rule-kill-action-by-executable", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 456, KillActionStatusPerformed)

		// Third kill should dismantle the rule
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 456, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(1), stats.killQueued)
		assert.Equal(t, int64(1), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("disarm-rule-kill-action-by-container", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 456, KillActionStatusPerformed)

		// Third kill should dismantle the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(1), stats.killQueued)
		assert.Equal(t, int64(1), stats.killQueuedDiscardedByDisarm)
	})
}

func TestProcessKillerNoDisarmers(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  1,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    false, // first disable the exec disarmer
			EnforcementDisarmerExecutableMaxAllowed: 1,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "process")

	t.Run("no-executable-disarmer-rule-to-disarm", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 456, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Third kill should be performed
		assertProcessKillEvent(t, pk, rule, "container1", "executable3", 789, KillActionStatusPerformed)

		// Fourth kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable3", 111, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("no-executable-disarmer-rule-to-dismantle", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 456, KillActionStatusQueued)

		// Third kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	// disable only the container disarmer
	cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled = false
	cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled = true

	t.Run("no-container-disarmer-rule-to-disarm", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Third kill should be performed
		assertProcessKillEvent(t, pk, rule, "container3", "executable1", 789, KillActionStatusPerformed)

		// Fourth kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container3", "executable2", 111, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("no-container-disarmer-rule-to-dismantle", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusQueued)

		// Third kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	// disable both disarmer
	cfg.RuntimeSecurity.EnforcementDisarmerContainerEnabled = false
	cfg.RuntimeSecurity.EnforcementDisarmerExecutableEnabled = false

	t.Run("no-container-nor-executable-disarmer-rule", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be performed
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusPerformed)

		// Second kill should be performed as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusPerformed)

		// Third kill should be performed as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusPerformed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(3), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(0), stats.killQueued)
		assert.Equal(t, int64(0), stats.killQueuedDiscardedByDisarm)
	})
}

func TestProcessKillerNoEnforcement(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      false,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  1,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 1,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "process")

	t.Run("no-enforcement-rule-kill-action-by-executable", func(t *testing.T) {
		pk.Reset(ruleSet)

		event := craftFakeEvent("container1", "executable1", 123)
		// First kill should be performed
		killed, _ := pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// reset event to different pid, but same container, same executable
		event = craftFakeEvent("container1", "executable1", 456)
		// Second kill should be performed as well
		killed, _ = pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// reset event to different pid AND executable
		event = craftFakeEvent("container1", "executable2", 789)
		// Third kill should be performed as well
		killed, _ = pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.Nil(t, stats)
	})

	t.Run("no-enforcement-rule-kill-action-by-container", func(t *testing.T) {
		pk.Reset(ruleSet)

		event := craftFakeEvent("container1", "executable1", 123)
		// First kill should be performed
		killed, _ := pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// reset event to different pid, but same container, same executable
		event = craftFakeEvent("container1", "executable1", 456)
		// Second kill should be performed as well
		killed, _ = pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// reset event to different pid AND container
		event = craftFakeEvent("container2", "executable1", 789)
		// Third kill should be performed as well
		killed, _ = pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.Nil(t, stats)
	})
}

func TestProcessKillerRuleNoDisarmers(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  1,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 1,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "process")
	rule.PolicyRule.Def.Actions[0].Kill.DisableExecutableDisarmer = true

	t.Run("no-executable-disarmer-by-rule-to-disarm", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 456, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Third kill should be performed
		assertProcessKillEvent(t, pk, rule, "container1", "executable3", 789, KillActionStatusPerformed)

		// Fourth kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable3", 111, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("no-executable-disarmer-by-rule-to-dismantle", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container1", "executable2", 456, KillActionStatusQueued)

		// Third kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	// disable only the container disarmer
	rule.PolicyRule.Def.Actions[0].Kill.DisableExecutableDisarmer = false
	rule.PolicyRule.Def.Actions[0].Kill.DisableContainerDisarmer = true

	t.Run("no-container-disarmer-by-rule-to-disarm", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Third kill should be performed
		assertProcessKillEvent(t, pk, rule, "container3", "executable1", 789, KillActionStatusPerformed)

		// Fourth kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container3", "executable2", 111, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(1), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	t.Run("no-container-disarmer-by-rule-to-dismantle", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusQueued)

		// Third kill should disarm the rule
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusRuleDismantled)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(2), stats.killQueuedDiscardedByDisarm)
	})

	// disable both disarmers
	rule.PolicyRule.Def.Actions[0].Kill.DisableExecutableDisarmer = true
	rule.PolicyRule.Def.Actions[0].Kill.DisableContainerDisarmer = true

	t.Run("no-container-nor-executable-disarmer-by-rule", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be performed
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusPerformed)

		// Second kill should be performed as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable1", 456, KillActionStatusPerformed)

		// Third kill should be performed as well
		assertProcessKillEvent(t, pk, rule, "container2", "executable2", 111, KillActionStatusPerformed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(3), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(0), stats.killQueued)
		assert.Equal(t, int64(0), stats.killQueuedDiscardedByDisarm)
	})
}

func TestProcessKillerRuleScopeContainer(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  1,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 1,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "container")

	t.Run("kill-container-rule-to-executable-disarm", func(t *testing.T) {
		pk.Reset(ruleSet)
		pk.vacumChan()

		// First kill should be enqueued
		assertContainerKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)

		// Second kill should be enqueued as well
		assertContainerKillEvent(t, pk, rule, "container1", "executable1", 456, KillActionStatusQueued)

		// trick the disarmer to be after the warmup period
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		disarmer.warmupEnd = time.Now().Add(-time.Second)

		// Third kill should perform 3 container kills
		assertContainerKillEvent(t, pk, rule, "container1", "executable1", 789, KillActionStatusPerformed)

		// Third kill should NOT perform any kill (because no container AND container scope)
		event := craftFakeEvent("", "executable1", 111)
		killed, _ := pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.False(t, killed)
		assert.Equal(t, 0, len(event.ActionReports))

		// Fourth kill should disarm the rule
		assertContainerKillEvent(t, pk, rule, "container1", "executable2", 222, KillActionStatusRuleDisarmed)

		// check stats
		stats := pk.getRuleStatsNoAlloc("test-rule")
		assert.NotNil(t, stats)
		assert.Equal(t, int64(3), stats.processesKilledDirectly)
		assert.Equal(t, int64(0), stats.processesKilledAfterQueue)
		assert.Equal(t, int64(2), stats.killQueued)
		assert.Equal(t, int64(6), stats.killQueuedDiscardedByDisarm)
	})
}

func TestIsKillAllowed(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		killContexts  []killContext
		excludedBins  []string
		expectedError string
		expectedAllow bool
	}{
		{
			name:          "enforcement disabled",
			enabled:       false,
			killContexts:  []killContext{{pid: 123}},
			expectedAllow: false,
			expectedError: "the enforcement capability is disabled",
		},
		{
			name:          "system process",
			enabled:       true,
			killContexts:  []killContext{{pid: 1}},
			expectedAllow: false,
			expectedError: "process with pid 1 cannot be killed",
		},
		{
			name:          "agent process",
			enabled:       true,
			killContexts:  []killContext{{pid: int(utils.Getpid())}},
			expectedAllow: false,
			expectedError: "process with pid " + strconv.FormatUint(uint64(utils.Getpid()), 10) + " cannot be killed",
		},
		{
			name:          "excluded binary",
			enabled:       true,
			killContexts:  []killContext{{pid: 123, path: "/usr/bin/protected"}},
			excludedBins:  []string{"/usr/bin/protected"},
			expectedAllow: false,
			expectedError: "process `/usr/bin/protected`(123) is protected",
		},
		{
			name:          "allowed process",
			enabled:       true,
			killContexts:  []killContext{{pid: 123, path: "/usr/bin/allowed"}},
			excludedBins:  []string{"/usr/bin/protected"},
			expectedAllow: true,
		},
		{
			name:    "multiple processes - one excluded",
			enabled: true,
			killContexts: []killContext{
				{pid: 123, path: "/usr/bin/allowed"},
				{pid: 456, path: "/usr/bin/protected"},
			},
			excludedBins:  []string{"/usr/bin/protected"},
			expectedAllow: false,
			expectedError: "process `/usr/bin/protected`(456) is protected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test config
			cfg := &config.Config{
				RuntimeSecurity: &config.RuntimeSecurityConfig{
					EnforcementRuleSourceAllowed: []string{"test"},
				},
			}
			pk, err := NewProcessKiller(cfg, nil)
			assert.NoError(t, err)

			// Set enabled state
			pk.SetState(tt.enabled)

			// Add excluded binaries
			for _, bin := range tt.excludedBins {
				glob, err := eval.NewGlob(bin, false, false)
				assert.NoError(t, err)
				pk.binariesExcluded = append(pk.binariesExcluded, glob)
			}

			// Test isKillAllowed
			allowed, err := pk.isKillAllowed(tt.killContexts)
			assert.Equal(t, tt.expectedAllow, allowed)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleProcessExitedWithPendingKills(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  5,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 5,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	t.Run("process-scope-single-pid-exit-aborts-report", func(t *testing.T) {
		pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
		assert.NoError(t, err)
		rule, ruleSet := craftKillRule(t, "test-rule", "process")
		pk.Reset(ruleSet)
		pk.vacumChan()

		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)
		assert.Equal(t, 1, len(pk.pendingReports))

		exitEvent := craftFakeEvent("container1", "executable1", 123)
		exitEvent.ProcessContext.ExitTime = time.Now()
		pk.HandleProcessExited(exitEvent)

		assert.Equal(t, 0, len(pk.pendingReports))
	})

	t.Run("container-scope-partial-exit-keeps-report-queued", func(t *testing.T) {
		pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
		assert.NoError(t, err)
		rule, ruleSet := craftKillRule(t, "test-rule", "container")
		pk.Reset(ruleSet)
		pk.vacumChan()

		// Container scope: FakeProcessKillerOS returns 3 PIDs (pid, pid+1, pid+2)
		assertContainerKillEvent(t, pk, rule, "container1", "executable1", 100, KillActionStatusQueued)
		assert.Equal(t, 1, len(pk.pendingReports))

		report := pk.pendingReports[0]
		report.RLock()
		assert.Equal(t, 3, len(report.pendingKills))
		report.RUnlock()

		// One PID exits: report should remain queued with 2 pending kills
		exitEvent := craftFakeEvent("container1", "executable1", 101)
		exitEvent.ProcessContext.ExitTime = time.Now()
		pk.HandleProcessExited(exitEvent)

		assert.Equal(t, 1, len(pk.pendingReports))
		report.RLock()
		assert.Equal(t, 2, len(report.pendingKills))
		assert.Equal(t, KillActionStatusQueued, report.Status)
		report.RUnlock()
	})

	t.Run("container-scope-all-pids-exit-aborts-report", func(t *testing.T) {
		pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
		assert.NoError(t, err)
		rule, ruleSet := craftKillRule(t, "test-rule", "container")
		pk.Reset(ruleSet)
		pk.vacumChan()

		assertContainerKillEvent(t, pk, rule, "container1", "executable1", 200, KillActionStatusQueued)
		assert.Equal(t, 1, len(pk.pendingReports))

		report := pk.pendingReports[0]

		// All 3 PIDs exit one by one
		for _, pid := range []uint32{200, 201, 202} {
			exitEvent := craftFakeEvent("container1", "executable1", pid)
			exitEvent.ProcessContext.ExitTime = time.Now()
			pk.HandleProcessExited(exitEvent)
		}

		assert.Equal(t, 0, len(pk.pendingReports))
		report.RLock()
		assert.Equal(t, KillActionStatusKillAborted, report.Status)
		assert.True(t, report.resolved)
		report.RUnlock()
	})
}

func TestAbortedReportNotOverwrittenByDisarmer(t *testing.T) {
	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:                      true,
			EnforcementDisarmerContainerEnabled:     true,
			EnforcementDisarmerContainerMaxAllowed:  5,
			EnforcementDisarmerContainerPeriod:      time.Second,
			EnforcementDisarmerExecutableEnabled:    true,
			EnforcementDisarmerExecutableMaxAllowed: 5,
			EnforcementDisarmerExecutablePeriod:     time.Second,
			EnforcementRuleSourceAllowed:            []string{"test"},
		},
	}

	t.Run("aborted-report-should-not-be-overwritten-by-killPendingForDisarmer", func(t *testing.T) {
		pk, err := NewProcessKiller(cfg, &FakeProcessKillerOS{})
		assert.NoError(t, err)
		rule, ruleSet := craftKillRule(t, "test-rule", "process")
		pk.Reset(ruleSet)
		pk.vacumChan()

		// Step 1: enqueue a kill during warmup
		assertProcessKillEvent(t, pk, rule, "container1", "executable1", 123, KillActionStatusQueued)
		assert.Equal(t, 1, len(pk.pendingReports))

		// Grab references
		report := pk.pendingReports[0]
		disarmer := pk.getDisarmer("test-rule")
		assert.NotNil(t, disarmer)
		assert.Equal(t, 1, len(disarmer.pendingReports))

		// Step 2: process exits -> report should become kill_aborted and be removed from p.pendingReports
		exitEvent := craftFakeEvent("container1", "executable1", 123)
		exitEvent.ProcessContext.ExitTime = time.Now()
		pk.HandleProcessExited(exitEvent)

		assert.Equal(t, 0, len(pk.pendingReports), "report should be removed from p.pendingReports")
		report.RLock()
		assert.Equal(t, KillActionStatusKillAborted, report.Status, "report should be kill_aborted")
		assert.True(t, report.resolved, "report should be resolved")
		report.RUnlock()

		// Step 3: simulate warmup end -> processPendingKills should NOT overwrite the aborted status
		disarmer.m.Lock()
		disarmer.pendingKillsAlarm = time.Now().Add(-time.Second)
		disarmer.m.Unlock()

		pk.processPendingKills()

		report.RLock()
		assert.Equal(t, KillActionStatusKillAborted, report.Status, "BUG: report status was overwritten from kill_aborted to something else by killPendingForDisarmer")
		assert.True(t, report.resolved, "report should still be resolved")
		report.RUnlock()
	})
}

func TestPartiallyPerformedStatus(t *testing.T) {
	failingPid := uint32(456)

	fakeOS := &FakeProcessKillerOSWithFailures{
		failPids: map[int]bool{int(failingPid): true},
	}

	cfg := &config.Config{
		RuntimeSecurity: &config.RuntimeSecurityConfig{
			EnforcementEnabled:           true,
			EnforcementRuleSourceAllowed: []string{"test"},
		},
	}

	pk, err := NewProcessKiller(cfg, fakeOS)
	assert.NoError(t, err)
	rule, ruleSet := craftKillRule(t, "test-rule", "container")
	pk.Reset(ruleSet)

	t.Run("partially-performed-when-one-pid-fails", func(t *testing.T) {
		// Container scope with PID 455: getProcesses returns [455, 456, 457]
		// PID 456 will fail to be killed
		event := craftFakeEvent("container1", "executable1", 455)
		killed, report := pk.KillAndReport(rule.PolicyRule.Def.Actions[0].Kill, rule, event)
		assert.True(t, killed)
		assert.NotNil(t, report)
		report.RLock()
		assert.Equal(t, KillActionStatusPartiallyPerformed, report.Status)
		report.RUnlock()
	})
}

type FakeProcessKillerOSWithFailures struct {
	failPids map[int]bool
}

func (fpk *FakeProcessKillerOSWithFailures) Kill(_ uint32, pc *killContext) error {
	if fpk.failPids[pc.pid] {
		return errors.New("fake kill failure")
	}
	return nil
}

func (fpk *FakeProcessKillerOSWithFailures) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error) {
	kcs := []killContext{
		{
			pid:  int(ev.ProcessContext.Pid),
			path: ev.ProcessContext.FileEvent.PathnameStr,
		},
	}
	if entry.Process.ContainerContext.ContainerID != "" && scope == "container" {
		kcs = append(kcs, []killContext{
			{
				pid:  int(ev.ProcessContext.Pid + 1),
				path: ev.ProcessContext.FileEvent.PathnameStr + "_1",
			},
			{
				pid:  int(ev.ProcessContext.Pid + 2),
				path: ev.ProcessContext.FileEvent.PathnameStr + "_2",
			},
		}...)
	}
	return kcs, nil
}
