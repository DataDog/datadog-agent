// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func getFamilyAddress(config *config.RuntimeSecurityConfig) (string, string) {
	return "unix", config.SocketPath
}

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts) {
	opts.ProbeOpts.PathResolutionEnabled = true
}

// DisableRuntimeSecurity disables all the runtime security features
func DisableRuntimeSecurity(config *config.Config) {
	config.Probe.NetworkEnabled = false
	config.RuntimeSecurity.ActivityDumpEnabled = false
	config.RuntimeSecurity.SecurityProfileEnabled = false
}

// platform specific init function
func (c *CWSConsumer) init(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts Opts) error {
	// Activity dumps related
	evm.Probe.AddActivityDumpHandler(c)

	return nil
}
