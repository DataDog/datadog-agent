// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts, config *config.Config) {
	opts.ProbeOpts.PathResolutionEnabled = true
	opts.ProbeOpts.TTYFallbackEnabled = true
	opts.ProbeOpts.SyscallsMonitorEnabled = config.Probe.SyscallsMonitorEnabled
	opts.ProbeOpts.EBPFLessEnabled = config.RuntimeSecurity.EBPFLessEnabled
}

// DisableRuntimeSecurity disables all the runtime security features
func DisableRuntimeSecurity(config *config.Config) {
	config.Probe.NetworkEnabled = false
	config.RuntimeSecurity.ActivityDumpEnabled = false
	config.RuntimeSecurity.SecurityProfileEnabled = false
}

// platform specific init function
func (c *CWSConsumer) init(evm *eventmonitor.EventMonitor, _ *config.RuntimeSecurityConfig, _ Opts) error {
	// Activity dumps related
	if p, ok := evm.Probe.PlatformProbe.(*probe.EBPFProbe); ok {
		p.AddActivityDumpHandler(c)
	}

	return nil
}
