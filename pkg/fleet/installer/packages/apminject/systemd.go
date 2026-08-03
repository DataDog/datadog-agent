// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apminject

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	systemdServiceName = "datadog-apm-inject.service"
	// installerPathPlaceholder is replaced in the embedded unit file with the
	// absolute path to the datadog-installer binary resolved at install time.
	installerPathPlaceholder = "{{INSTALLER_PATH}}"
)

// installerPathCandidates lists the locations the static datadog-installer
// binary may live in.
var installerPathCandidates = []string{
	"/opt/datadog-packages/datadog-installer/stable/bin/installer/installer",
	"/opt/datadog-packages/run/datadog-installer-ssi",
	"/opt/datadog-packages/datadog-agent/stable/embedded/bin/installer",
	"/usr/bin/datadog-installer",
}

//go:embed datadog-apm-inject.service
var apmInjectServiceFile []byte

// SystemdServiceManager manages the APM injector systemd service
type SystemdServiceManager struct {
	servicePath   string
	serviceName   string
	installerPath string
}

// NewSystemdServiceManager builds a manager pointing at the first on-disk
// datadog-installer that supports the `apm instrument-start`/`instrument-stop`
// subcommands the unit invokes. The resolved path is baked into the unit's
// ExecStart/ExecStop. installerPath is "" when no supported installer is found
// (no candidate on disk, or only older ones); callers must then skip rendering
// the unit and fall back to direct ld.so.preload management (see
// setupSystemdPreloadUnit), since the candidate set is not guaranteed in practice.
func NewSystemdServiceManager() *SystemdServiceManager {
	installerPath, err := resolveInstallerPath(installerPathCandidates, supportsInstrumentSubcommands)
	if err != nil {
		log.Warnf("no datadog-installer supporting `apm instrument-start` found for APM inject service: %v", err)
	}
	return &SystemdServiceManager{
		servicePath:   filepath.Join(systemd.UserUnitsPath, systemdServiceName),
		serviceName:   systemdServiceName,
		installerPath: installerPath,
	}
}

// InstallerPath returns the supported datadog-installer path resolved at
// construction time, or "" if none was found.
func (s *SystemdServiceManager) InstallerPath() string {
	return s.installerPath
}

// ServiceFileExists reports whether the unit file has been written to disk.
func (s *SystemdServiceManager) ServiceFileExists() bool {
	_, err := os.Stat(s.servicePath)
	return err == nil
}

// installerVerifier reports whether the datadog-installer at path supports the
// subcommands the unit invokes. It is a parameter of resolveInstallerPath so
// tests can exercise pure path resolution without an executable installer.
type installerVerifier func(path string) bool

// supportsInstrumentSubcommands reports whether the installer at path exposes
// `apm instrument-start` and `apm instrument-stop`. It inspects the `apm --help`
// usage text rather than the exit status (which varies by command tree); the
// subcommands are not Hidden, so a supporting installer lists both.
func supportsInstrumentSubcommands(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "apm", "--help").CombinedOutput()
	supported := bytes.Contains(out, []byte("instrument-start")) && bytes.Contains(out, []byte("instrument-stop"))
	if !supported {
		log.Debugf("installer candidate %s does not expose apm instrument-start/stop (err: %v)", path, err)
	}
	return supported
}

// Setup writes the embedded service file, enables it for future boots, and
// starts it immediately. Returns an error if any step fails, including the
// immediate start: a unit that cannot start is removed by the caller.
func (s *SystemdServiceManager) Setup(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "systemd_service_setup")
	defer func() { span.Finish(err) }()
	span.SetTag("installer_path", s.installerPath)

	// failed_step records which step failed so the fallback cause (start vs.
	// write/reload/enable) is visible in the trace without the caller having to
	// re-derive it from the returned error.
	if err := s.writeServiceFile(); err != nil {
		span.SetTag("failed_step", "write_file")
		return err
	}
	log.Infof("Installed systemd service file at %s (installer: %s)", s.servicePath, s.installerPath)

	if err := systemd.Reload(ctx); err != nil {
		span.SetTag("failed_step", "reload")
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if err := systemd.EnableUnit(ctx, s.serviceName); err != nil {
		span.SetTag("failed_step", "enable")
		return fmt.Errorf("failed to enable systemd service: %w", err)
	}

	if err := systemd.StartUnit(ctx, s.serviceName); err != nil {
		span.SetTag("failed_step", "start")
		return fmt.Errorf("failed to start systemd service: %w", err)
	}
	log.Infof("APM injector systemd service installed, enabled, and started")
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

	if s.installerPath == "" {
		return fmt.Errorf("refusing to write %s with empty installer path", s.servicePath)
	}
	content := bytes.ReplaceAll(apmInjectServiceFile, []byte(installerPathPlaceholder), []byte(s.installerPath))

	if err := os.WriteFile(s.servicePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file at %s: %w", s.servicePath, err)
	}
	return nil
}

func resolveInstallerPath(candidates []string, verify installerVerifier) (string, error) {
	var skippedUnsupported []string
	for _, p := range candidates {
		info, err := os.Stat(p)
		// Skip if doesn't exist.
		if err != nil {
			continue
		}
		// Skip if dir or not executable.
		if info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		// Skip binaries too old to support the subcommands the unit invokes, so we
		// never bake a doomed ExecStart into the service file and fall through to a
		// supported candidate if one exists.
		if verify != nil && !verify(p) {
			skippedUnsupported = append(skippedUnsupported, p)
			continue
		}
		return p, nil
	}
	if len(skippedUnsupported) > 0 {
		return "", fmt.Errorf("found datadog-installer binaries but none support `apm instrument-start` (too old): %v", skippedUnsupported)
	}
	return "", fmt.Errorf("no datadog-installer binary found among %v", candidates)
}
