// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultKillActionFlushDelay = 2 * time.Second
)

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
	opts.normalize()

	p := &Probe{
		Opts:         opts,
		Config:       config,
		StatsdClient: opts.StatsdClient,
		scrubber:     newProcScrubber(config.Probe.CustomSensitiveWords),
	}

	if opts.EBPFLessEnabled {
		pp, err := NewEBPFLessProbe(p, config, opts)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
	} else {
		pp, err := NewEBPFProbe(p, config, opts)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
	}

	return p, nil
}

func handleKillActions(action *rules.ActionDefinition, ev *model.Event, kill func(pid uint32, sig uint32) error) *KillActionReport {
	entry, exists := ev.ResolveProcessCacheEntry()
	if !exists {
		return nil
	}

	var pids []uint32

	if entry.ContainerID != "" && action.Kill.Scope == "container" {
		pids = entry.GetContainerPIDs()
	} else {
		pids = []uint32{ev.ProcessContext.Pid}
	}

	sig := model.SignalConstants[action.Kill.Signal]

	killedAt := time.Now()
	for _, pid := range pids {
		if pid <= 1 || pid == utils.Getpid() {
			continue
		}

		log.Debugf("requesting signal %s to be sent to %d", action.Kill.Signal, pid)

		if err := kill(uint32(pid), uint32(sig)); err != nil {
			seclog.Debugf("failed to kill process %d: %s", pid, err)
		}
	}

	report := &KillActionReport{
		Signal:     action.Kill.Signal,
		Pid:        ev.ProcessContext.Pid,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		KilledAt:   killedAt,
	}

	ev.ActionReports = append(ev.ActionReports, report)

	return report
}
