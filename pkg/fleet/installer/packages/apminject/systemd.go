// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apminject

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	systemdServiceName = "datadog-apm-inject.service"
)

//go:embed datadog-apm-inject.service
var apmInjectServiceFile []byte

// SystemdServiceManager manages the APM injector systemd service
type SystemdServiceManager struct {
	servicePath string
	serviceName string
}

// NewSystemdServiceManager creates a new SystemdServiceManager
func NewSystemdServiceManager() *SystemdServiceManager {
	return &SystemdServiceManager{
		servicePath: filepath.Join(systemd.UserUnitsPath, systemdServiceName),
		serviceName: systemdServiceName,
	}
}

// Setup writes the embedded service file and enables it for future boots.
// It also attempts to start the service immediately, but a start failure is
// non-fatal: the service is still enabled and will start on the next boot.
// The caller is expected to call InstrumentLDPreload directly to cover the
// current boot in case the service did not start.
func (s *SystemdServiceManager) Setup(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "systemd_service_setup")
	defer func() { span.Finish(err) }()

	if err := s.writeServiceFile(); err != nil {
		return err
	}
	log.Infof("Installed systemd service file at %s", s.servicePath)

	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if err := systemd.EnableUnit(ctx, s.serviceName); err != nil {
		return fmt.Errorf("failed to enable systemd service: %w", err)
	}

	if err := systemd.StartUnit(ctx, s.serviceName); err != nil {
		// Non-fatal: the service is enabled and will start on next boot.
		// The caller will fall back to direct ld.so.preload instrumentation
		// for the current boot.
		log.Warnf("APM inject service failed to start immediately (will start on next boot): %v", err)
	} else {
		log.Infof("APM injector systemd service installed, enabled, and started")
	}
	return nil
}

// Uninstall stops, disables, and removes the APM injector systemd service
func (s *SystemdServiceManager) Uninstall(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "systemd_service_uninstall")
	defer func() { span.Finish(err) }()

	if err := systemd.StopUnit(ctx, s.serviceName); err != nil {
		log.Warnf("Failed to stop systemd service (may not be running): %v", err)
	}

	if err := systemd.DisableUnit(ctx, s.serviceName); err != nil {
		log.Warnf("Failed to disable systemd service: %v", err)
	}

	if err := os.Remove(s.servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove systemd service file: %w", err)
	}

	if err := systemd.Reload(ctx); err != nil {
		// Non-fatal: the service file is already removed. daemon-reload is best-effort
		// cleanup; if systemd is unreachable the stale reference resolves on next reload.
		log.Warnf("Failed to reload systemd daemon after uninstall (ignored): %v", err)
	}

	log.Infof("APM injector systemd service uninstalled successfully")
	return nil
}

func (s *SystemdServiceManager) writeServiceFile() error {
	if err := os.MkdirAll(filepath.Dir(s.servicePath), 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %w", err)
	}

	if err := os.WriteFile(s.servicePath, apmInjectServiceFile, 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file at %s: %w", s.servicePath, err)
	}
	return nil
}
