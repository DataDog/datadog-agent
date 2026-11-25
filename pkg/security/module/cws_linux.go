// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts, config *config.Config) {
	if config.RuntimeSecurity.IsRuntimeEnabled() {
		opts.ProbeOpts.PathResolutionEnabled = true
		opts.ProbeOpts.TTYFallbackEnabled = true
		opts.ProbeOpts.SyscallsMonitorEnabled = config.Probe.SyscallsMonitorEnabled
		opts.ProbeOpts.EBPFLessEnabled = config.RuntimeSecurity.EBPFLessEnabled
	} else {
		DisableRuntimeSecurity(config)
		if pkgconfigsetup.Datadog().GetBool("sbom.enrichment.usage.enabled") {
			opts.ProbeOpts.PathResolutionEnabled = true
		}
	}
}

// DisableRuntimeSecurity disables all the runtime security features
func DisableRuntimeSecurity(config *config.Config) {
	config.Probe.NetworkEnabled = false
	config.RuntimeSecurity.ActivityDumpEnabled = false
	config.RuntimeSecurity.SecurityProfileEnabled = false
	config.RuntimeSecurity.SecurityProfileV2Enabled = false
	config.RuntimeSecurity.SysCtlEnabled = false
	// The task storage syscall cache allocates on every hooked syscall entry and frees on exit; the LRU hash costs
	// less per syscall. Only the default changes: an explicit setting is kept.
	if !syscallTaskStorageConfigured() {
		config.Probe.EventStreamUseSyscallTaskStorage = false
	}
}

func syscallTaskStorageConfigured() bool {
	sp := pkgconfigsetup.SystemProbe()
	return sp.IsConfigured("event_monitoring_config.event_stream.use_syscall_task_storage") ||
		sp.IsConfigured("runtime_security_config.event_stream.use_syscall_task_storage")
}

// platform specific init function
func (c *CWSConsumer) init(evm *eventmonitor.EventMonitor, _ *config.RuntimeSecurityConfig, _ Opts) error {
	// Activity dumps related
	if p, ok := evm.Probe.PlatformProbe.(*probe.EBPFProbe); ok {
		p.AddActivityDumpHandler(c)
	}

	return nil
}
