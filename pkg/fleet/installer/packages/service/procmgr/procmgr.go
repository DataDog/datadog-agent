// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package procmgr provides a set of functions to manage dd-procmgrd, the Datadog process manager.
//
// dd-procmgrd is itself hosted by a systemd unit, so unit-level operations delegate to the systemd
// package. The payloads it supervises are declared as one YAML file per process in the install
// root's processes.d directory, which is what DD_PM_CONFIG_DIR points the daemon at.
package procmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ProcessesDirName is the per-install directory holding one YAML per supervised process.
	// It's the equivalent of systemd's .service files, but for processes managed by procmgr.
	ProcessesDirName = "processes.d"

	daemonRelPath = "embedded/bin/dd-procmgrd"
	cliRelPath    = "embedded/bin/dd-procmgr"

	cliTimeout = 120 * time.Second
)

var (
	// installRoots are the trees the installer may manage. deb/rpm installs use
	// /opt/datadog-agent, OCI installs use the datadog-agent repository links.
	installRoots = []string{
		"/opt/datadog-agent",
		filepath.Join(paths.PackagesPath, "datadog-agent", "stable"),
		filepath.Join(paths.PackagesPath, "datadog-agent", "experiment"),
	}

	// socketPath matches RuntimeDirectory=datadog-procmgrd in datadog-agent-procmgr.service and
	// DEFAULT_SOCKET_PATH in pkg/procmgr/rust. Its presence is the cheapest reliable "daemon is
	// up" signal: systemd removes RuntimeDirectory when the unit stops.
	socketPath = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
)

// IsInstalled reports whether dd-procmgrd exists under any known install root. The service manager
// selection uses it so hosts whose Agent predates dd-procmgrd, or flavors that do not ship it, keep
// the systemd manager rather than selecting one that cannot run.
func IsInstalled() bool {
	for _, root := range installRoots {
		fi, err := os.Stat(filepath.Join(root, daemonRelPath))
		if err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// ConfigDir returns the processes.d directory for an install root.
func ConfigDir(installRoot string) string {
	return filepath.Join(installRoot, ProcessesDirName)
}

// WriteConfig writes a single processes.d entry, creating the directory if needed.
func WriteConfig(installRoot string, name string, content []byte) error {
	dir := ConfigDir(installRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// RemoveConfigs removes processes.d entries. Missing entries are not an error.
func RemoveConfigs(installRoot string, names ...string) error {
	var errs error
	for _, name := range names {
		err := os.Remove(filepath.Join(ConfigDir(installRoot), name))
		if err != nil && !os.IsNotExist(err) {
			errs = multierr.Append(errs, err)
		}
	}
	return errs
}

// ListConfigs returns the names of the processes.d entries present under an install root. A missing
// directory yields no entries and no error.
func ListConfigs(installRoot string) ([]string, error) {
	entries, err := os.ReadDir(ConfigDir(installRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yaml" && ext != ".yml" {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// Reload asks a running dd-procmgrd to re-read processes.d. An entry whose file was deleted is
// stopped, one whose file changed is stopped and respawned, a new one is started. An unchanged
// entry is left alone, so a reload does not re-evaluate condition_path_exists: use a unit restart
// when only the payload's presence on disk has changed.
//
// No-op when the daemon is not running or the CLI is absent.
func Reload(ctx context.Context, installRoot string) error {
	_, err := runCLI(ctx, installRoot, "reload")
	return err
}

// EnableUnit enables the systemd unit hosting dd-procmgrd or one of its peers.
func EnableUnit(ctx context.Context, unit string) error {
	return systemd.EnableUnit(ctx, unit)
}

// DisableUnits disables multiple units.
func DisableUnits(ctx context.Context, units ...string) error {
	return systemd.DisableUnits(ctx, units...)
}

// StartUnit starts a unit.
func StartUnit(ctx context.Context, unit string) error {
	return systemd.StartUnit(ctx, unit)
}

// StopUnits stops multiple units. Stopping the unit hosting dd-procmgrd also stops the processes it
// supervises: the daemon shuts them down in reverse startup order.
func StopUnits(ctx context.Context, units ...string) error {
	return systemd.StopUnits(ctx, units...)
}

// RestartUnit restarts a unit. Restarting the Agent's main unit cycles dd-procmgrd through its
// BindsTo/Wants relationship, and the fresh daemon re-reads every definition and re-evaluates
// condition_path_exists.
func RestartUnit(ctx context.Context, unit string) error {
	return systemd.RestartUnit(ctx, unit)
}

// cliPath returns <installRoot>/embedded/bin/dd-procmgr after confirming the joined path resolves
// structurally under installRoot, so argv0 is never attacker-influenced.
func cliPath(installRoot string) (string, error) {
	if !filepath.IsAbs(installRoot) {
		return "", fmt.Errorf("install root %q is not absolute", installRoot)
	}
	root := filepath.Clean(installRoot)
	cli := filepath.Clean(filepath.Join(root, cliRelPath))
	rel, err := filepath.Rel(root, cli)
	if err != nil {
		return "", fmt.Errorf("dd-procmgr path layout: %w", err)
	}
	if filepath.ToSlash(rel) != cliRelPath {
		return "", errors.New("unexpected dd-procmgr path layout")
	}
	return cli, nil
}

// runCLI invokes dd-procmgr. It reports ran=false with a nil error when there is nothing to talk to
// — no daemon socket or no CLI on disk — so callers can treat that as success.
func runCLI(ctx context.Context, installRoot string, args ...string) (ran bool, err error) {
	if _, err := os.Stat(socketPath); err != nil {
		log.Infof("Installer: dd-procmgrd socket %s is absent, skipping %v", socketPath, args)
		return false, nil
	}
	cli, err := cliPath(installRoot)
	if err != nil {
		log.Infof("Installer: no usable dd-procmgr CLI under %s (%v), skipping %v", installRoot, err, args)
		return false, nil
	}
	if _, err := os.Stat(cli); err != nil {
		log.Infof("Installer: dd-procmgr CLI %s is absent, skipping %v", cli, args)
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()
	// argv0 is confined to <installRoot>/embedded/bin/dd-procmgr by cliPath.
	// no-dd-sa:go-security/command-injection
	if err := telemetry.CommandContext(ctx, cli, args...).Run(); err != nil {
		return true, fmt.Errorf("dd-procmgr %v: %w", args, err)
	}
	return true, nil
}
