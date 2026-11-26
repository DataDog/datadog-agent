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
	// ddProcmgrReloadRefusedExitCode is the exit code of `dd-procmgr reload` when the daemon refused
	// the reload. It must match RELOAD_REFUSED_EXIT_CODE in pkg/procmgr/rust/src/bins/dd-procmgr.rs.
	ddProcmgrReloadRefusedExitCode = 3
)

// isReloadRefused reports whether a failed `dd-procmgr reload` was refused by the daemon, as
// opposed to failing to run or to reach it.
func isReloadRefused(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == ddProcmgrReloadRefusedExitCode
}

// reloadFailureAction is what to do after `dd-procmgr reload` failed.
type reloadFailureAction int

const (
	// restartService restarts dd-procmgr-service, which rereads processes.d from scratch.
	restartService reloadFailureAction = iota
	// stopProcess stops only the named process and leaves the service and the rest running.
	stopProcess
	// leaveRunning does nothing more.
	leaveRunning
)

// reloadFailureActionFor picks the action after a failed reload. A refused reload means the
// daemon could not read processes.d and kept its current processes. A restart would read the same
// directory and start from whatever it yields, so it is not used then: the named process, if any,
// is stopped on its own and everything else keeps running.
func reloadFailureActionFor(err error, processToStopIfRefused string) reloadFailureAction {
	if !isReloadRefused(err) {
		return restartService
	}
	if processToStopIfRefused == "" {
		return leaveRunning
	}
	return stopProcess
}

// stopProcmgrProcess runs `dd-procmgr stop <name>`, which returns once the process has stopped.
// A fresh timeout is used so a slow reload cannot starve the stop.
func stopProcmgrProcess(cli, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), ddProcmgrReloadOrRestartTimeout)
	defer cancel()
	// argv0 is constrained to <DatadogProgramFilesDir>\bin\agent\dd-procmgr.exe (validatedDDProcmgrCLI).
	// no-dd-sa:go-security/command-injection
	out, err := exec.CommandContext(ctx, cli, "stop", name).CombinedOutput()
	if err != nil {
		log.Warnf("DDOT: dd-procmgr stop %s failed (%v); output: %s", name, err, strings.TrimSpace(string(out)))
		return
	}
	log.Infof("DDOT: stopped %s through dd-procmgr", name)
}

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
//
// When the daemon refuses the reload (it could not read processes.d and kept its current
// processes), the service is not restarted. Instead the process named by processToStopIfRefused,
// which the caller needs stopped, is stopped with `dd-procmgr stop`, and everything else keeps
// running. An empty name leaves all processes running.
func ReloadOrRestartProcmgr(processToStopIfRefused string) {
	if paths.DatadogProgramFilesDir == "" {
		log.Warnf("DDOT: DatadogProgramFilesDir is empty; cannot reload or restart %s", ddProcmgrServiceName)
		return
	}
	running, err := winutil.IsServiceRunning(ddProcmgrServiceName)
	if err != nil {
		log.Warnf("DDOT: could not query %s state before reload: %v", ddProcmgrServiceName, err)
		return
	}
	if !running {
		log.Debugf("DDOT: skip reload/restart; %s is not running", ddProcmgrServiceName)
		return
	}
	cli, pathErr := validatedDDProcmgrCLI()
	if pathErr != nil {
		log.Warnf("DDOT: invalid dd-procmgr path (%v); falling back to %s restart", pathErr, ddProcmgrServiceName)
		if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err != nil {
			log.Warnf("DDOT: failed to restart %s after removing DDOT process manager config (invalid install path): %v", ddProcmgrServiceName, err)
		}
		return
	}
	if _, err := os.Stat(cli); err != nil {
		if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err != nil {
			log.Warnf("DDOT: failed to restart %s after removing DDOT process manager config (no dd-procmgr CLI): %v", ddProcmgrServiceName, err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddProcmgrReloadOrRestartTimeout)
	defer cancel()
	// argv0 is constrained to <DatadogProgramFilesDir>\bin\agent\dd-procmgr.exe (validatedDDProcmgrCLI).
	// no-dd-sa:go-security/command-injection
	cmd := exec.CommandContext(ctx, cli, "reload")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	switch reloadFailureActionFor(err, processToStopIfRefused) {
	case stopProcess:
		log.Warnf("DDOT: dd-procmgr refused the reload and kept its current processes (output: %s); stopping %s only", strings.TrimSpace(string(out)), processToStopIfRefused)
		stopProcmgrProcess(cli, processToStopIfRefused)
	case leaveRunning:
		log.Warnf("DDOT: dd-procmgr refused the reload and kept its current processes; output: %s", strings.TrimSpace(string(out)))
	default:
		log.Warnf("DDOT: dd-procmgr reload failed (%v); output: %s; falling back to %s restart", err, strings.TrimSpace(string(out)), ddProcmgrServiceName)
		if err2 := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrReloadOrRestartTimeout); err2 != nil {
			log.Warnf("DDOT: failed to restart %s after removing DDOT process manager config: %v", ddProcmgrServiceName, err2)
		}
	}
}

// removeEmptyProcessesDir drops installPF\processes.d after fleet prerm removed the last YAML.
// MSI uninstall cleanup removes the empty directory before deleting the install root.
func removeEmptyProcessesDir(installPF string) {
	if installPF == "" {
		return
	}
	processesDir := filepath.Join(installPF, "processes.d")
	entries, err := os.ReadDir(processesDir)
	if err != nil {
		return
	}
	if len(entries) > 0 {
		return
	}
	if err := os.Remove(processesDir); err != nil && !os.IsNotExist(err) {
		log.Debugf("processes.d: remove empty dir %q: %v", processesDir, err)
	}
}
