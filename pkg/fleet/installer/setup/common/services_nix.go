// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package common

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// restartServices restarts the services that need to be restarted after a package upgrade or
// an install script re-run; because the configuration may have changed.
func (s *Setup) restartServices(pkgs []packageWithVersion) error {
	t := time.Now()
	span, ctx := telemetry.StartSpanFromContext(s.Ctx, "restartServices")
	for _, pkg := range pkgs {
		switch pkg.name {
		case DatadogAgentPackage:
			err := systemd.RestartUnit(ctx, "datadog-agent.service")
			if err != nil {
				logs, logsErr := systemd.JournaldLogs(ctx, "datadog-agent.service", t)
				span.SetTag("journald_logs", logs)
				span.SetTag("journald_logs_err", logsErr)
				return fmt.Errorf("failed to restart datadog-agent.service: %w", err)
			}
		}
	}
	return nil
}
