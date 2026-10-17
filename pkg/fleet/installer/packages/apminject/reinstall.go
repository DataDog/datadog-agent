// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apminject

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// hostWiring is the on-disk instrumentation state a reinstall decision is made
// on. It is read up front so the decision itself stays a pure function.
type hostWiring struct {
	ldSoPreload  []byte
	dockerDaemon []byte
	// dockerInstalled mirrors what Instrument uses to decide whether it touches
	// daemon.json at all, so a host without docker is never seen as broken.
	dockerInstalled bool
	// installerPath is the datadog-installer the apm-inject systemd unit would
	// run, empty when no on-disk installer supports the unit, and
	// tmpfsCompatible whether that installer knows the tmpfs preload lifecycle.
	installerPath   string
	tmpfsCompatible bool
}

// RequiresReinstall reports whether an already-installed injector package must
// have its post-install hook replayed.
//
// The package database only records which payload sits on disk, so a package
// that registers as installed can still leave the host uninstrumented:
// `dd-host-install --uninstall` and `dd-container-install --uninstall` revert
// the host wiring without touching the package, and a stale tmpfs preload entry
// outlives a downgrade to an installer that no longer recreates the symlink on
// boot. Replaying the hook repairs all of these.
func RequiresReinstall(ctx context.Context) (required bool) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "requires_reinstall")
	defer func() { span.Finish(nil) }()

	mgr := NewSystemdServiceManager()
	required, reason := requiresReinstall(NewInstaller(), hostWiring{
		ldSoPreload:     readFileBestEffort(ldSoPreloadPath),
		dockerDaemon:    readFileBestEffort(dockerDaemonPath),
		dockerInstalled: isDockerInstalled(ctx),
		installerPath:   mgr.InstallerPath(),
		tmpfsCompatible: mgr.TmpfsCompatible(),
	})
	span.SetTag("required", required)
	span.SetTag("reason", reason)
	if required {
		log.Infof("APM injector is installed but %s, replaying its install hook", reason)
	}
	return required
}

// requiresReinstall is the decision behind RequiresReinstall. The reason it
// returns is empty when no reinstall is needed, and is reported in telemetry
// and logs otherwise.
func requiresReinstall(a *InjectorInstaller, w hostWiring) (bool, string) {
	if shouldInstrumentHost(a.Env) && !a.isHostInstrumented(w.ldSoPreload) {
		return true, "host injection is missing from " + ldSoPreloadPath
	}
	if shouldInstrumentDocker(a.Env) && w.dockerInstalled && !a.isDockerInstrumented(w.dockerDaemon) {
		return true, "the injector runtime is missing from " + dockerDaemonPath
	}
	if w.installerPath != "" && requiresReinstallForPreload(string(w.ldSoPreload), w.tmpfsCompatible) {
		return true, "its tmpfs preload entry outlives the installer that recreates it"
	}
	return false, ""
}

func readFileBestEffort(path string) []byte {
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("could not read %s while checking the APM injector state: %v", path, err)
		}
		return nil
	}
	return content
}
