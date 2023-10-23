// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package module

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UpdateEventMonitorOpts adapt the event monitor option
func UpdateEventMonitorOpts(opts *eventmonitor.Opts, config *config.Config) { //nolint:revive // TODO fix revive unused-parameter
	if config.RuntimeSecurity.RemoteConfigurationEnabled {
		log.Info("remote-configuration is not supported for CWS on this platform")
		// force disabling RC on non linux platform
		config.RuntimeSecurity.RemoteConfigurationEnabled = false
	}
}

// DisableRuntimeSecurity disables all the runtime security features
func DisableRuntimeSecurity(config *config.Config) {} //nolint:revive // TODO fix revive unused-parameter

// platform specific init function
func (c *CWSConsumer) init(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts Opts) error { //nolint:revive // TODO fix revive unused-parameter
	return nil
}
