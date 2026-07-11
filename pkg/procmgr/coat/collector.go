// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"errors"
	"os"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		Services: make([]ServiceSnapshot, 0, len(migratableServices)),
	}

	var processes map[string]ProcessSnapshot

	sess, err := c.client.Connect(ctx)
	if err != nil {
		logCoatProcmgrErr("coat: dd-procmgrd connect", err)
		snapshot.Daemon = DaemonSnapshot{}
		processes = map[string]ProcessSnapshot{}
	} else {
		defer func() { _ = sess.Disconnect() }()
		snapshot.Daemon, err = sess.Status(ctx)
		if err != nil {
			logCoatProcmgrErr("coat: dd-procmgrd status", err)
			snapshot.Daemon = DaemonSnapshot{}
			processes = map[string]ProcessSnapshot{}
		} else {
			processes, err = sess.List(ctx)
			if err != nil {
				logCoatProcmgrErr("coat: dd-procmgrd list", err)
				processes = map[string]ProcessSnapshot{}
			}
		}
	}

	for _, service := range migratableServices {
		snapshot.Services = append(snapshot.Services, c.collectService(ctx, service, processes))
	}
	return snapshot
}

// logCoatProcmgrErr logs expected "procmgr not there / not ready" conditions at debug and
// everything else at warn so hosts without dd-procmgrd are not noisy by default.
func logCoatProcmgrErr(msg string, err error) {
	if debugProcmgrClientErr(err) {
		log.Debugf("%s: %v", msg, err)
		return
	}
	log.Warnf("%s: %v", msg, err)
}

// debugProcmgrClientErr reports whether err is an expected client condition (missing socket,
// refused connection, shutdown/cancel, common gRPC transport states) and should be logged at debug.
func debugProcmgrClientErr(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable:
			return true
		}
	}
	return false
}

func (c *Collector) collectService(ctx context.Context, service MigratableService, processes map[string]ProcessSnapshot) ServiceSnapshot {
	status := ServiceSnapshot{
		ID:             service.ID,
		ManagementMode: ManagementModeNone,
	}

	for _, marker := range installMarkerPaths(c.installRoot, service) {
		if marker == "" {
			continue
		}
		if _, err := os.Stat(marker); err == nil {
			status.Installed = true
			break
		}
	}

	if _, err := os.Stat(procmgrConfigPath(c.installRoot, service.ProcmgrConfigFile)); err == nil {
		status.ProcmgrConfigured = true
	}

	if process, ok := processes[service.ProcmgrProcessName]; ok {
		status.ProcmgrState = process.State
		status.ManagementMode = ManagementModeProcmgr
		return status
	}

	if legacyMode := detectLegacySupervisor(ctx, service); legacyMode != ManagementModeNone {
		// Install marker may be missing for pre-extension installs that still run under
		// systemd/SCM; treat legacy supervision as sufficient evidence the service is installed.
		status.Installed = true
		status.ManagementMode = legacyMode
	}

	return status
}
