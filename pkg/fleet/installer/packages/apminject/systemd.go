// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	systemdServiceName = "datadog-apm-injector.service"
	systemdServicePath = "/etc/systemd/system/datadog-apm-injector.service"
	apmInjectorBinPath = "/opt/datadog-packages/datadog-apm-inject/stable/bin/apm-injector"
)

// SystemdServiceManager manages the APM injector systemd service
type SystemdServiceManager struct {
	binPath     string
	servicePath string
	serviceName string
}

// NewSystemdServiceManager creates a new SystemdServiceManager
func NewSystemdServiceManager() *SystemdServiceManager {
	return &SystemdServiceManager{
		binPath:     apmInjectorBinPath,
		servicePath: systemdServicePath,
		serviceName: systemdServiceName,
	}
}

// Install creates and enables the APM injector systemd service
func (s *SystemdServiceManager) Install(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "systemd_service_install")
	defer func() { span.Finish(err) }()

	// Check if apm-injector binary exists
	if _, err := os.Stat(s.binPath); err != nil {
		return fmt.Errorf("apm-injector binary not found at %s: %w", s.binPath, err)
	}

	// Create the service file
	serviceContent := fmt.Sprintf(`[Unit]
Description=Datadog APM Injector
Documentation=https://docs.datadoghq.com/
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s install
ExecStop=%s uninstall
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, s.binPath, s.binPath)

	if err := os.WriteFile(s.servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	log.Infof("Created systemd service file at %s", s.servicePath)

	// Reload systemd to pick up the new service
	if err := s.systemctlDaemonReload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable the service
	if err := s.systemctlEnable(ctx); err != nil {
		return fmt.Errorf("failed to enable systemd service: %w", err)
	}

	// Start the service
	if err := s.systemctlStart(ctx); err != nil {
		return fmt.Errorf("failed to start systemd service: %w", err)
	}

	log.Infof("APM injector systemd service installed and started successfully")
	return nil
}

// Uninstall stops, disables, and removes the APM injector systemd service
func (s *SystemdServiceManager) Uninstall(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "systemd_service_uninstall")
	defer func() { span.Finish(err) }()

	// Stop the service if it's running
	if err := s.systemctlStop(ctx); err != nil {
		log.Warnf("Failed to stop systemd service (may not be running): %v", err)
	}

	// Disable the service
	if err := s.systemctlDisable(ctx); err != nil {
		log.Warnf("Failed to disable systemd service: %v", err)
	}

	// Remove the service file
	if err := os.Remove(s.servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove systemd service file: %w", err)
	}

	// Reload systemd
	if err := s.systemctlDaemonReload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	log.Infof("APM injector systemd service uninstalled successfully")
	return nil
}

// IsInstalled checks if the systemd service is installed
func (s *SystemdServiceManager) IsInstalled() bool {
	_, err := os.Stat(s.servicePath)
	return err == nil
}

// systemctlDaemonReload reloads systemd configuration
func (s *SystemdServiceManager) systemctlDaemonReload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w, output: %s", err, string(output))
	}
	return nil
}

// systemctlEnable enables the systemd service
func (s *SystemdServiceManager) systemctlEnable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "enable", s.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl enable failed: %w, output: %s", err, string(output))
	}
	return nil
}

// systemctlDisable disables the systemd service
func (s *SystemdServiceManager) systemctlDisable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "disable", s.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl disable failed: %w, output: %s", err, string(output))
	}
	return nil
}

// systemctlStart starts the systemd service
func (s *SystemdServiceManager) systemctlStart(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "start", s.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl start failed: %w, output: %s", err, string(output))
	}
	return nil
}

// systemctlStop stops the systemd service
func (s *SystemdServiceManager) systemctlStop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "stop", s.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl stop failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetBinaryPath returns the path where the apm-injector binary should be installed
func GetAPMInjectorBinaryPath() string {
	return filepath.Join(injectorPath, "bin", "apm-injector")
}
