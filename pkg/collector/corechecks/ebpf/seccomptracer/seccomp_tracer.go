// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package seccomptracer contains the Seccomp Tracer check
package seccomptracer

import (
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	secmodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "seccomp_tracer"

	// Seccomp action masks and values from <linux/seccomp.h>
	seccompRetActionFull = 0xffff0000
	seccompRetData       = 0x0000ffff

	seccompRetKillProcess = 0x80000000
	seccompRetKillThread  = 0x00000000
	seccompRetTrap        = 0x00030000
	seccompRetErrno       = 0x00050000
	seccompRetTrace       = 0x7ff00000
	seccompRetLog         = 0x7ffc0000
	seccompRetAllow       = 0x7fff0000
)

// SeccompTracerCheck grabs seccomp failure metrics
type SeccompTracerCheck struct {
	core.CheckBase
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &SeccompTracerCheck{
		CheckBase: core.NewCheckBase(CheckName),
		tagger:    tagger,
	}
}

// Configure parses the check configuration and init the check
func (t *SeccompTracerCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := t.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return fmt.Errorf("failed to configure check: %w", err)
	}
	t.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))

	return nil
}

// syscallName returns the human-readable name for a syscall number
func syscallName(nr uint32) string {
	// Use the security model's syscall name lookup
	name := secmodel.Syscall(nr).String()
	// If the secmodel returns the number back, it's unknown
	if name == fmt.Sprintf("Syscall(%d)", nr) {
		return fmt.Sprintf("syscall_%d", nr)
	}
	return name
}

// seccompActionName returns the human-readable name for a seccomp action
func seccompActionName(action uint32) string {
	actionType := action & seccompRetActionFull
	actionData := action & seccompRetData

	switch actionType {
	case seccompRetKillProcess:
		return "kill_process"
	case seccompRetKillThread:
		return "kill_thread"
	case seccompRetTrap:
		return "trap"
	case seccompRetErrno:
		// Include the errno value for errno actions
		if actionData > 0 {
			// Try to get the errno name
			errName := unix.ErrnoName(unix.Errno(actionData))
			if errName != "" {
				return fmt.Sprintf("errno_%s", errName)
			}
			return fmt.Sprintf("errno_%d", actionData)
		}
		return "errno"
	case seccompRetTrace:
		return "trace"
	case seccompRetLog:
		return "log"
	case seccompRetAllow:
		return "allow"
	default:
		return fmt.Sprintf("action_0x%08x", action)
	}
}

// Run executes the check
func (t *SeccompTracerCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[model.SeccompStats](t.sysProbeClient, sysconfig.SeccompTracerModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	sender, err := t.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender: %w", err)
	}

	for _, v := range stats {
		containerID, err := cgroups.ContainerFilter("", v.CgroupName)
		if err != nil || containerID == "" {
			log.Debugf("Unable to extract containerID from cgroup name: %s, err: %v", v.CgroupName, err)
		}

		entityID := types.NewEntityID(types.ContainerID, containerID)
		var tags []string
		if !entityID.Empty() {
			tags, err = t.tagger.Tag(entityID, types.HighCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", v.CgroupName, err)
			}
		}

		// Add syscall and action as human-readable tags
		tags = append(tags, fmt.Sprintf("syscall_nr:%d", v.SyscallNr))
		tags = append(tags, fmt.Sprintf("syscall_name:%s", syscallName(v.SyscallNr)))
		tags = append(tags, fmt.Sprintf("seccomp_action:%s", seccompActionName(v.SeccompAction)))

		sender.Gauge("seccomp.denials", float64(v.Count), "", tags)
	}

	sender.Commit()
	return nil
}
