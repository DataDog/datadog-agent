// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	ddProcmgrServiceName = "dd-procmgr-service"
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

// ReloadOrRestartProcmgr tells dd-procmgrd to re-read processes.d from disk (e.g. after DDOT
// processes.d removal). Prefer `dd-procmgr reload` over an SCM service restart; fall back to
// restarting dd-procmgr-service when the CLI is absent or reload fails. No-op when the service
// is already stopped (e.g. MSI prerm runs after StopDDServices).
func ReloadOrRestartProcmgr() {
	if paths.DatadogProgramFilesDir == "" {
		log.Warnf("procmgr: DatadogProgramFilesDir is empty; cannot reload or restart %s", ddProcmgrServiceName)
		return
	}
	running, err := winutil.IsServiceRunning(ddProcmgrServiceName)
	if err != nil {
		log.Warnf("procmgr: could not query %s state before reload: %v", ddProcmgrServiceName, err)
		return
	}
	if !running {
		log.Debugf("procmgr: skip reload/restart; %s is not running", ddProcmgrServiceName)
		return
	}
	cli, pathErr := validatedDDProcmgrCLI()
	if pathErr != nil {
		log.Warnf("procmgr: invalid dd-procmgr path (%v); falling back to %s restart", pathErr, ddProcmgrServiceName)
		RestartProcmgrService()
		return
	}
	if _, err := os.Stat(cli); err != nil {
		RestartProcmgrService()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddProcmgrReloadOrRestartTimeout)
	defer cancel()
	// argv0 is constrained to <DatadogProgramFilesDir>\bin\agent\dd-procmgr.exe (validatedDDProcmgrCLI).
	// no-dd-sa:go-security/command-injection
	cmd := exec.CommandContext(ctx, cli, "reload")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf(
			"procmgr: dd-procmgr reload failed (%v); output: %s; falling back to %s restart",
			err,
			strings.TrimSpace(string(out)),
			ddProcmgrServiceName,
		)
		RestartProcmgrService()
	}
}

// RestartProcmgrService restarts dd-procmgr-service so it re-reads processes.d from disk.
// No-op when the service is already stopped (e.g. MSI prerm runs after StopDDServices).
func RestartProcmgrService() {
	if paths.DatadogProgramFilesDir == "" {
		log.Warnf("procmgr: DatadogProgramFilesDir is empty; cannot restart %s", ddProcmgrServiceName)
		return
	}
	running, err := winutil.IsServiceRunning(ddProcmgrServiceName)
	if err != nil {
		log.Warnf("procmgr: could not query %s state before restart: %v", ddProcmgrServiceName, err)
		return
	}
	if !running {
		log.Debugf("procmgr: skip restart; %s is not running", ddProcmgrServiceName)
		return
	}
	if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err != nil {
		log.Warnf("procmgr: failed to restart %s after processes.d change: %v", ddProcmgrServiceName, err)
	}
}
