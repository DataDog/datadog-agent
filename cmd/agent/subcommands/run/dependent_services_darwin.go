// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package run

import (
	"context"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Must match procmgrJob in pkg/fleet/installer/packages/datadog_agent_darwin.go.
const (
	procmgrLaunchdLabel  = "com.datadoghq.procmgr"
	procmgrServiceTarget = "system/" + procmgrLaunchdLabel
	procmgrPlistPath     = "/Library/LaunchDaemons/" + procmgrLaunchdLabel + ".plist"
)

// launchctl runs /bin/launchctl. Tests substitute a fake.
var launchctl = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
}

// Aligns com.datadoghq.procmgr with process_manager.enabled when the Agent starts.
// When enabled: clear any disabled override, bootstrap the plist if unloaded, then kickstart.
// When disabled: disable then bootout so KeepAlive / RunAtLoad cannot bring it back.
// Does not create a missing plist: that stays install-time only.
func startDependentServicesAsync(coreConf model.Reader, _ model.Reader) {
	go syncProcmgrWithConfig(coreConf)
}

func syncProcmgrWithConfig(coreConf model.Reader) {
	ctx := context.Background()
	if coreConf.GetBool("process_manager.enabled") {
		enableProcmgr(ctx)
		return
	}
	disableProcmgr(ctx)
}

func enableProcmgr(ctx context.Context) {
	if out, err := launchctl(ctx, "enable", procmgrServiceTarget); err != nil {
		log.Debugf("could not enable %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
	// Bootstrap loads the plist if a prior disable path booted it out. No-op when already loaded.
	if out, err := launchctl(ctx, "bootstrap", "system", procmgrPlistPath); err != nil {
		log.Debugf("could not bootstrap %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
	if out, err := launchctl(ctx, "kickstart", procmgrServiceTarget); err != nil {
		log.Debugf("could not kickstart %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
}

func disableProcmgr(ctx context.Context) {
	// Disable first so KeepAlive / RunAtLoad cannot restart the job after bootout, including across reboot.
	if out, err := launchctl(ctx, "disable", procmgrServiceTarget); err != nil {
		log.Debugf("could not disable %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
	if out, err := launchctl(ctx, "bootout", procmgrServiceTarget); err != nil {
		log.Debugf("could not bootout %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
}

// launchd owns procmgr during Agent stop. The enabled kill switch is enforced on the next start
// via syncProcmgrWithConfig.
func stopDependentServices(_ model.Reader, _ model.Reader) {}
