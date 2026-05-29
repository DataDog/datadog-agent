// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"fmt"
	"strings"
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

// lookbackCheckRunner wraps an independent Runner+Scheduler pair that runs
// a configurable set of core checks and writes their output directly to the
// lookback WAL via a custom walSenderManager (no aggregator involved).
type lookbackCheckRunner struct {
	r   *runner.Runner
	s   *scheduler.Scheduler
	log log.Component
}

// newLookbackCheckRunner creates and starts the runner. Returns nil (not an
// error) when lookback.checks is empty — the runner is disabled.
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

	r := runner.NewRunner(senderMgr, &noopHaAgent{}, &noopHealthPlatform{})
	s := scheduler.NewScheduler(r.GetChan())
	r.SetScheduler(s)

	cr := &lookbackCheckRunner{r: r, s: s, log: l}

	// Build YAML instance config that sets the desired collection interval.
	intervalSecs := int(interval.Seconds())
	if intervalSecs < 1 {
		intervalSecs = 1
	}
	instanceData := integration.Data(fmt.Sprintf("min_collection_interval: %d", intervalSecs))

	// Built-in check factories: bypass the global catalog (which is populated
	// later by commonchecks.RegisterChecks) and instantiate directly.
	cpuOpt    := cpu.Factory()
	loadOpt   := load.Factory()
	memoryOpt := memory.Factory()
	factories := map[string]func() check.Check{}
	if f, ok := cpuOpt.Get(); ok {
		factories[cpu.CheckName] = f
	}
	if f, ok := loadOpt.Get(); ok {
		factories[load.CheckName] = f
	}
	if f, ok := memoryOpt.Get(); ok {
		factories[memory.CheckName] = f
	}

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
		if err := s.Enter(chk); err != nil {
			l.Warnf("lookback check runner: schedule %q failed: %v", name, err)
			continue
		}
		l.Infof("lookback check runner: scheduled %q every %s", name, interval)
	}

	s.Run()
	return cr
}

func (cr *lookbackCheckRunner) stop() {
	if cr == nil {
		return
	}
	if err := cr.s.Stop(); err != nil {
		cr.log.Warnf("lookback check runner: scheduler stop: %v", err)
	}
	cr.r.Stop()
}

// --- Minimal noops for Runner dependencies ---

// noopHaAgent satisfies haagent.Component. IsActive returns true so checks
// always run; all other methods are no-ops or return zero values.
type noopHaAgent struct{}

func (n *noopHaAgent) Enabled() bool                   { return false }
func (n *noopHaAgent) GetConfigID() string              { return "" }
func (n *noopHaAgent) GetState() haagent.State          { return haagent.Unknown }
func (n *noopHaAgent) SetLeader(_ string)               {}
func (n *noopHaAgent) IsActive() bool                   { return true }

// noopHealthPlatform satisfies healthplatformdef.Component with all no-ops.
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
func (n *noopHealthPlatform) ResolveIssue(_ string)    {}
func (n *noopHealthPlatform) ResolveAllIssues()         {}
