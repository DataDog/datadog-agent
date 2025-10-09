// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// startDependentServices is a no-op on Linux/macOS.
// On these platforms, dependent services (process-agent, trace-agent, etc.) are managed
// by systemd/launchd independently and are not started by the core agent.
//
// However, we log information about infrastructure basic mode to help users understand
// which services should be disabled when using basic mode.
func startDependentServices() {
	if pkgconfigsetup.Datadog().GetString("infrastructure_mode") == "basic" {
		log.Info("Infrastructure basic mode enabled - only the core agent should run")
		log.Warn("IMPORTANT: On Linux/macOS, the core agent service file has 'Wants=' directives")
		log.Warn("that automatically start dependent services. You must MASK (not just disable) them:")
		log.Info("")
		log.Info("  sudo systemctl mask datadog-agent-trace")
		log.Info("  sudo systemctl mask datadog-agent-process")
		log.Info("  sudo systemctl mask datadog-agent-sysprobe")
		log.Info("  sudo systemctl mask datadog-agent-security")
		log.Info("")
		log.Warn("Using 'disable' is NOT enough - you must use 'mask' to prevent auto-start")
		log.Info("For more information, see: https://docs.datadoghq.com/agent/basic_agent_usage/")
	}
}

// stopDependentServices is a no-op on Linux/macOS.
// On these platforms, dependent services are managed by systemd/launchd independently.
func stopDependentServices() {}
