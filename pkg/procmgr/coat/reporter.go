// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"runtime"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

const reportInterval = 5 * time.Minute

type gauges struct {
	daemonReachable          telemetry.Gauge
	daemonReady              telemetry.Gauge
	processRunning           telemetry.Gauge
	serviceInstalled         telemetry.Gauge
	serviceProcmgrConfigured telemetry.Gauge
	serviceManagementMode    telemetry.Gauge
}

// StartReporter periodically probes dd-procmgrd and migratable services, updating COAT gauges.
func StartReporter(ctx context.Context, tlm telemetry.Component) {
	if tlm == nil {
		return
	}

	g := gauges{
		daemonReachable: tlm.NewGauge("runtime", "procmgr_daemon_reachable", []string{}, "dd-procmgrd is reachable from the core agent"),
		daemonReady:     tlm.NewGauge("runtime", "procmgr_daemon_ready", []string{}, "dd-procmgrd reports ready"),
		processRunning:  tlm.NewGauge("runtime", "procmgr_process_running", []string{"process"}, "Managed process is running under dd-procmgrd"),
		serviceInstalled: tlm.NewGauge(
			"runtime",
			"agent_service_installed",
			[]string{"service"},
			"Agent service payload is installed on the host",
		),
		serviceProcmgrConfigured: tlm.NewGauge(
			"runtime",
			"agent_service_procmgr_configured",
			[]string{"service"},
			"Agent service has a processes.d config for dd-procmgrd",
		),
		serviceManagementMode: tlm.NewGauge(
			"runtime",
			"agent_service_management_mode",
			[]string{"service", "mode"},
			"How an agent service process is supervised",
		),
	}

	collector := NewCollector()
	report(ctx, g, collector)
	go runReporterLoop(ctx, g, collector)
}

func runReporterLoop(ctx context.Context, g gauges, collector *Collector) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	// DDOT install/upgrade can restart procmgr and the agent shortly after the
	// initial snapshot; refresh a few times before the regular 5-minute cadence.
	for _, delay := range []time.Duration{30 * time.Second, 2 * time.Minute} {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			report(ctx, g, collector)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report(ctx, g, collector)
		}
	}
}

func report(ctx context.Context, g gauges, collector *Collector) {
	snapshot := collector.Collect(ctx)

	setBoolGauge(g.daemonReachable, snapshot.Daemon.Reachable)
	setBoolGauge(g.daemonReady, snapshot.Daemon.Ready)

	for _, service := range snapshot.Services {
		spec, ok := serviceByID(service.ID)
		if !ok {
			continue
		}

		setBoolGauge(g.serviceInstalled, service.Installed, service.ID)
		setBoolGauge(g.serviceProcmgrConfigured, service.ProcmgrConfigured, service.ID)
		setBoolGauge(g.processRunning, service.ProcmgrState == pb.ProcessState_RUNNING, spec.ProcmgrProcessName)

		// Do not emit management_mode=none on platforms where we never classify
		// systemd/SCM/procmgr (e.g. macOS); avoids polluting COAT adoption metrics.
		emitMgmtMode := service.ManagementMode != ManagementModeNone ||
			runtime.GOOS == "linux" || runtime.GOOS == "windows"
		if emitMgmtMode {
			for _, mode := range managementModes {
				setBoolGauge(g.serviceManagementMode, service.ManagementMode == mode, service.ID, string(mode))
			}
		}
	}
}

func setBoolGauge(g telemetry.Gauge, value bool, tags ...string) {
	if g == nil {
		return
	}
	if value {
		g.Set(1, tags...)
		return
	}
	g.Set(0, tags...)
}
