// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package module

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

// UpdateEventMonitorOpts adapt the event monitor option
func UpdateEventMonitorOpts(_ *eventmonitor.Opts, _ *config.Config) {}

// DisableRuntimeSecurity disables all the runtime security features
func DisableRuntimeSecurity(_ *config.Config) {}

// platform specific init function
func (c *CWSConsumer) init(_ *eventmonitor.EventMonitor, _ *config.RuntimeSecurityConfig, _ Opts) error {
	return nil
}
