// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/windowsunits"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var ddProcmgrServiceName = windowsunits.SCM.Procmgr.Name

const (
	// ddProcmgrReloadOrRestartTimeout bounds `dd-procmgr reload` and SCM restart after processes.d changes.
	ddProcmgrReloadOrRestartTimeout = 120 * time.Second
)

// validatedDDProcmgrCLI returns the absolute path to dd-procmgr.exe under DatadogProgramFilesDir
// only when it resolves structurally to <installRoot>\bin\agent\dd-procmgr.exe.
func validatedDDProcmgrCLI() (string, error) {
	raw := paths.DatadogProgramFilesDir
	if raw == "" {
		return "", errors.New("DatadogProgramFilesDir is empty")
	}
	root := filepath.Clean(raw)
	if root == "." {
		return "", errors.New("DatadogProgramFilesDir is invalid")
	}
	cli := filepath.Join(root, "bin", "agent", "dd-procmgr.exe")
	cli = filepath.Clean(cli)
	wantRel := filepath.Join("bin", "agent", "dd-procmgr.exe")
	rel, err := filepath.Rel(root, cli)
	if err != nil {
		return "", fmt.Errorf("dd-procmgr path layout: %w", err)
	}
	if !strings.EqualFold(filepath.ToSlash(rel), filepath.ToSlash(wantRel)) {
		return "", errors.New("unexpected dd-procmgr path layout")
	}
	return cli, nil
}

// reloadOrRestartProcmgr tells dd-procmgrd to re-read processes.d from disk. Prefer `dd-procmgr reload` over an SCM service restart; fall back to
// restarting dd-procmgr-service when the CLI is absent or reload fails. No-op when the service
// is already stopped (e.g. MSI prerm runs after StopDDServices).
func reloadOrRestartProcmgr() {
	if paths.DatadogProgramFilesDir == "" {
		log.Warnf("processes.d: DatadogProgramFilesDir is empty; cannot reload or restart %s", ddProcmgrServiceName)
		return
	}
	running, err := winutil.IsServiceRunning(ddProcmgrServiceName)
	if err != nil {
		log.Warnf("processes.d: could not query %s state before reload: %v", ddProcmgrServiceName, err)
		return
	}
	if !running {
		log.Debugf("processes.d: skip reload/restart; %s is not running", ddProcmgrServiceName)
		return
	}
	cli, pathErr := validatedDDProcmgrCLI()
	if pathErr != nil {
		log.Warnf("processes.d: invalid dd-procmgr path (%v); falling back to %s restart", pathErr, ddProcmgrServiceName)
		if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err != nil {
			log.Warnf("processes.d: failed to restart %s after a processes.d change (invalid install path): %v", ddProcmgrServiceName, err)
		}
		return
	}
	if _, err := os.Stat(cli); err != nil {
		if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err != nil {
			log.Warnf("processes.d: failed to restart %s after a processes.d change (no dd-procmgr CLI): %v", ddProcmgrServiceName, err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddProcmgrReloadOrRestartTimeout)
	defer cancel()
	// argv0 is constrained to <DatadogProgramFilesDir>\bin\agent\dd-procmgr.exe (validatedDDProcmgrCLI).
	// no-dd-sa:go-security/command-injection
	cmd := exec.CommandContext(ctx, cli, "reload")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("processes.d: dd-procmgr reload failed (%v); output: %s; falling back to %s restart", err, strings.TrimSpace(string(out)), ddProcmgrServiceName)
		if err2 := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err2 != nil {
			log.Warnf("processes.d: failed to restart %s after a processes.d change: %v", ddProcmgrServiceName, err2)
		}
		return
	}
}

// writeProcmgrConfig renders u with installRoot into processesDir. It is skipped when u.Binary is
// not installed under installRoot.
func writeProcmgrConfig(u units.Unit, installRoot, processesDir string) error {
	binary := filepath.Join(installRoot, filepath.FromSlash(u.Binary))
	if _, err := os.Stat(binary); err != nil {
		log.Debugf("processes.d: skip writing %s (stat %s: %v)", u.Name, binary, err)
		return nil
	}
	content, err := windowsunits.Render(u, installRoot, paths.DatadogDataDir)
	if err != nil {
		return err
	}
	log.Debugf("processes.d: writing %s in %s", u.Name, processesDir)
	return units.WriteFile(processesDir, u.Name, content)
}

// removeProcmgrConfigs removes us from processesDir, and processesDir if that leaves it empty.
// MSI uninstall cleanup removes the empty directory before deleting the install root.
func removeProcmgrConfigs(processesDir string, us ...units.Unit) error {
	return errors.Join(units.RemoveFiles(processesDir, units.Names(us, false)...), units.RemoveDirIfEmpty(processesDir))
}

func procmgrConfigDir(installRoot string) string {
	return filepath.Join(installRoot, "processes.d")
}
