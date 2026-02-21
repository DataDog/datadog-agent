// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// restartServices restarts the services that need to be restarted after a package upgrade or
// an install script re-run; because the configuration may have changed.
func (s *Setup) restartServices(ctx context.Context, pkgs []packageWithVersion) (err error) {
	t := time.Now()
	span, ctx := telemetry.StartSpanFromContext(ctx, "restartServices")
	defer func() { span.Finish(err) }()
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

// stopServices stops the services that need to be stopped before running the installer
func (s *Setup) stopServices(_ context.Context, _ []packageWithVersion) error {
	// Not necessary on Linux, services are stopped in preinst hook
	return nil
}
