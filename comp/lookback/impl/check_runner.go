// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/load"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// minSchedulerInterval is the minimum interval supported by the scheduler.
// For shorter intervals we use a direct ticker loop instead.
const minSchedulerInterval = time.Second

// lookbackCheckRunner wraps either a Runner+Scheduler pair (interval ≥ 1s)
// or a direct ticker loop (interval < 1s) to run core checks and write their
// output to the lookback WAL.
type lookbackCheckRunner struct {
	// used for ≥1s path
	r   *runner.Runner
	s   *scheduler.Scheduler
	// used for <1s path
	cancel context.CancelFunc
	wg     sync.WaitGroup
	log    log.Component
}

// newLookbackCheckRunner creates and starts the runner. Returns nil when
// lookback.checks is empty.
func newLookbackCheckRunner(
	checkNames []string,
	interval time.Duration,
	store *shardedStore,
	ctxFile *contextFile,
	l log.Component,
) *lookbackCheckRunner {
	if len(checkNames) == 0 {
		return nil
	}

	senderMgr := newWALSenderManager(store, ctxFile, l)

	// Built-in check factories (bypasses the global catalog).
	cpuOpt    := cpu.Factory()
	loadOpt   := load.Factory()
	memoryOpt := memory.Factory()
	factories := map[string]func() check.Check{}
	if f, ok := cpuOpt.Get(); ok    { factories[cpu.CheckName] = f }
	if f, ok := loadOpt.Get(); ok   { factories[load.CheckName] = f }
	if f, ok := memoryOpt.Get(); ok { factories[memory.CheckName] = f }

	// Build YAML instance config. For the sub-second path the interval is
	// driven by the ticker, so we use 1s (the minimum accepted by Configure).
	configSecs := int(interval.Seconds())
	if configSecs < 1 {
		configSecs = 1
	}
	instanceData := integration.Data(fmt.Sprintf("min_collection_interval: %d", configSecs))

	var checks []check.Check
	for _, name := range checkNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		factory, ok := factories[name]
		if !ok {
			l.Warnf("lookback check runner: check %q not supported (available: cpu, memory, load)", name)
			continue
		}
		chk := factory()
		if err := chk.Configure(senderMgr, 0, instanceData, integration.Data{}, "lookback", "lookback"); err != nil {
			l.Warnf("lookback check runner: configure %q failed: %v", name, err)
			continue
		}
		checks = append(checks, chk)
		l.Infof("lookback check runner: scheduled %q every %s", name, interval)
	}

	if len(checks) == 0 {
		return nil
	}

	cr := &lookbackCheckRunner{log: l}

	if interval >= minSchedulerInterval {
		// Standard path: Runner + Scheduler (≥1s intervals).
		r := runner.NewRunner(senderMgr, &noopHaAgent{}, &noopHealthPlatform{})
		s := scheduler.NewScheduler(r.GetChan())
		r.SetScheduler(s)
		cr.r = r
		cr.s = s
		for _, chk := range checks {
			if err := s.Enter(chk); err != nil {
				l.Warnf("lookback check runner: enter %q: %v", chk, err)
			}
		}
		s.Run()
	} else {
		// Sub-second path: direct ticker loop, bypasses scheduler minimum.
		ctx, cancel := context.WithCancel(context.Background())
		cr.cancel = cancel
		for _, chk := range checks {
			cr.wg.Add(1)
			go func(c check.Check) {
				defer cr.wg.Done()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := c.Run(); err != nil {
							l.Debugf("lookback check runner: %s run error: %v", c, err)
						}
						// walSender.Commit() is called by the check itself.
					}
				}
			}(chk)
		}
	}

	return cr
}

func (cr *lookbackCheckRunner) stop() {
	if cr == nil {
		return
	}
	// Stop sub-second ticker goroutines.
	if cr.cancel != nil {
		cr.cancel()
		cr.wg.Wait()
	}
	// Stop scheduler + runner.
	if cr.s != nil {
		if err := cr.s.Stop(); err != nil {
			cr.log.Warnf("lookback check runner: scheduler stop: %v", err)
		}
	}
	if cr.r != nil {
		cr.r.Stop()
	}
}

// --- Minimal noops for Runner dependencies ---

type noopHaAgent struct{}

func (n *noopHaAgent) Enabled() bool                   { return false }
func (n *noopHaAgent) GetConfigID() string              { return "" }
func (n *noopHaAgent) GetState() haagent.State          { return haagent.Unknown }
func (n *noopHaAgent) SetLeader(_ string)               {}
func (n *noopHaAgent) IsActive() bool                   { return true }

type noopHealthPlatform struct{}

func (n *noopHealthPlatform) ReportIssue(_ string, _ string, _ *healthplatformpayload.IssueReport) error {
	return nil
}
func (n *noopHealthPlatform) ScheduleHealthCheck(_ string, _ string, _ healthplatformdef.HealthCheckFunc, _ time.Duration) error {
	return nil
}
func (n *noopHealthPlatform) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	return 0, nil
}
func (n *noopHealthPlatform) GetIssue(_ string) *healthplatformpayload.Issue { return nil }
func (n *noopHealthPlatform) ResolveIssue(_ string)                           {}
func (n *noopHealthPlatform) ResolveAllIssues()                               {}
