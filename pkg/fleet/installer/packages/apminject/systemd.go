// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	systemdServiceName        = "datadog-ssi-starter.service"
	injectorServiceSourcePath = injectorPath + "/systemd/datadog-ssi-starter.service"
	ssiBinPath                = injectorPath + "/inject/ssi-starter"
)

// HasDaemonStylePackage reports whether the installed OCI package is the daemon-style
// one that ships the ssi-starter binary and a bundled systemd service file.
// It returns an error if the package is in an inconsistent state (only one of
// the two expected files is present).
func HasDaemonStylePackage() (bool, error) {
	_, ssiErr := os.Stat(ssiBinPath)
	_, svcErr := os.Stat(injectorServiceSourcePath)

	ssiPresent := ssiErr == nil
	svcPresent := svcErr == nil

	if ssiPresent && svcPresent {
		log.Infof("APM inject: daemon-style package detected (ssi-starter at %s, systemd service at %s)", ssiBinPath, injectorServiceSourcePath)
		return true, nil
	}
	if !ssiPresent && !svcPresent {
		log.Infof("APM inject: daemon-style package not present, falling back to legacy installer mode")
		return false, nil
	}
	// Inconsistent state: exactly one of the two files is present.
	return false, fmt.Errorf("APM inject: inconsistent package state: ssi-starter present=%v (%s), systemd service present=%v (%s)",
		ssiPresent, ssiBinPath, svcPresent, injectorServiceSourcePath)
}

// SystemdServiceManager manages the APM injector systemd service
type SystemdServiceManager struct {
	serviceSourcePath string
	servicePath       string
	serviceName       string
}

// NewSystemdServiceManager creates a new SystemdServiceManager
func NewSystemdServiceManager() *SystemdServiceManager {
	return &SystemdServiceManager{
		serviceSourcePath: injectorServiceSourcePath,
		servicePath:       filepath.Join(systemd.UserUnitsPath, systemdServiceName),
		serviceName:       systemdServiceName,
	}
}

// Install copies the service file from the OCI package and enables the service
func (s *SystemdServiceManager) Install(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "systemd_service_install")
	defer func() { span.Finish(err) }()

	if err := s.copyServiceFile(); err != nil {
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
		return fmt.Errorf("failed to start systemd service: %w", err)
	}

	log.Infof("APM injector systemd service installed and started successfully")
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
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	log.Infof("APM injector systemd service uninstalled successfully")
	return nil
}

func (s *SystemdServiceManager) copyServiceFile() error {
	src, err := os.Open(s.serviceSourcePath)
	if err != nil {
		return fmt.Errorf("failed to open service file from OCI package at %s: %w", s.serviceSourcePath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(s.servicePath), 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %w", err)
	}

	dst, err := os.OpenFile(s.servicePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create systemd service file at %s: %w", s.servicePath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy service file: %w", err)
	}
	return nil
}
