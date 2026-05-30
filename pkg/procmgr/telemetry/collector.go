// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"os"
)

// Collector gathers procmgr daemon and service supervision snapshots.
type Collector struct {
	installRoot string
	client      Client
}

// NewCollector creates a collector using the default dd-procmgr gRPC client.
func NewCollector() *Collector {
	installRoot := agentInstallRoot()
	return NewCollectorWithClient(installRoot, newDefaultClient())
}

// NewCollectorWithClient creates a collector with a custom install root and procmgr client.
func NewCollectorWithClient(installRoot string, client Client) *Collector {
	return &Collector{
		installRoot: installRoot,
		client:      client,
	}
}

// Collect returns the current procmgr daemon and service supervision snapshot.
func (c *Collector) Collect(ctx context.Context) Snapshot {
	ctx, cancel := clientContext(ctx)
	defer cancel()

	snapshot := Snapshot{
		Daemon:   c.collectDaemon(ctx),
		Services: make([]ServiceSnapshot, 0, len(migratableServices)),
	}

	processes, _ := c.client.ListProcesses(ctx)
	for _, service := range migratableServices {
		snapshot.Services = append(snapshot.Services, c.collectService(service, processes))
	}
	return snapshot
}

func (c *Collector) collectDaemon(ctx context.Context) DaemonSnapshot {
	status, err := c.client.DaemonStatus(ctx)
	if err != nil {
		return DaemonSnapshot{}
	}
	return status
}

func (c *Collector) collectService(service MigratableService, processes map[string]ProcessSnapshot) ServiceSnapshot {
	status := ServiceSnapshot{
		ID:             service.ID,
		ManagementMode: ManagementModeNone,
	}

	if _, err := os.Stat(installMarkerPath(c.installRoot, service.InstallMarkerRel)); err != nil {
		return status
	}
	status.Installed = true

	if _, err := os.Stat(procmgrConfigPath(c.installRoot, service.ProcmgrConfigFile)); err == nil {
		status.ProcmgrConfigured = true
	}

	if process, ok := processes[service.ProcmgrProcessName]; ok {
		status.ProcmgrState = process.State
		status.ManagementMode = ManagementModeProcmgr
		return status
	}

	if legacyMode := detectLegacySupervisor(service); legacyMode != ManagementModeNone {
		status.ManagementMode = legacyMode
	}

	return status
}
