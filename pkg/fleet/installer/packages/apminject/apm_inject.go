// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package apminject implements the apm injector installer
package apminject

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/multierr"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	injectorPath          = "/opt/datadog-packages/datadog-apm-inject/stable"
	ldSoPreloadPath       = "/etc/ld.so.preload"
	oldLauncherPath       = "/opt/datadog/apm/inject/launcher.preload.so"
	localStableConfigPath = "/etc/datadog-agent/application_monitoring.yaml"

	// launcherPatternSuffix is the regex fragment that matches the launcher
	// filename with an optional intermediate subdir (inherited from earlier code).
	launcherPatternSuffix = `/(.*?/)?launcher\.preload\.so`

	// defaultTmpfsInjectDir is a symlink living on tmpfs (/run is a tmpfs on
	// systemd hosts, wiped on every boot) that points at the persistent
	// injector payload under injectorPath.
	defaultTmpfsInjectDir = "/run/datadog-apm-inject"
)

// systemdServiceManager is the subset of *SystemdServiceManager that
// setupSystemdPreloadUnit depends on. Declared as an interface so tests can
// inject a fake and exercise the fallback branches without real systemd.
type systemdServiceManager interface {
	InstallerPath() string
	ServiceFileExists() bool
	Setup(ctx context.Context) error
	Uninstall(ctx context.Context) error
}

// NewInstaller returns a new APM injector installer
func NewInstaller() *InjectorInstaller {
	a := &InjectorInstaller{
		installPath:    injectorPath,
		tmpfsInjectDir: defaultTmpfsInjectDir,
		Env:            env.FromEnv(),
	}
	a.ldPreloadFileInstrument = newFileMutator(ldSoPreloadPath, a.setLDPreloadConfigContent, nil, nil)
	a.ldPreloadFileUninstrument = newFileMutator(ldSoPreloadPath, a.deleteLDPreloadConfigContent, nil, nil)
	a.dockerConfigInstrument = newFileMutator(dockerDaemonPath, a.setDockerConfigContent, nil, nil)
	a.dockerConfigUninstrument = newFileMutator(dockerDaemonPath, a.deleteDockerConfigContent, nil, nil)
	return a
}

// InjectorInstaller installs the APM injector
type InjectorInstaller struct {
	installPath               string
	ldPreloadFileInstrument   *fileMutator
	ldPreloadFileUninstrument *fileMutator
	dockerConfigInstrument    *fileMutator
	dockerConfigUninstrument  *fileMutator
	Env                       *env.Env

	// tmpfsInjectDir is the tmpfs symlink directory used to reference the
	// launcher in a reboot-safe way. Defaults to defaultTmpfsInjectDir;
	// overridable in tests.
	tmpfsInjectDir string
	// launcherPath is the path written to /etc/ld.so.preload. Empty until
	// resolved; ldPreloadEntry falls back to the persistent OCI path.
	launcherPath string

	rollbacks []func() error
	cleanups  []func()
}

// Finish cleans up the APM injector
// Runs rollbacks if an error is passed and always runs cleanups
func (a *InjectorInstaller) Finish(err error) {
	if err != nil {
		// Run rollbacks in reverse order
		for i := len(a.rollbacks) - 1; i >= 0; i-- {
			if a.rollbacks[i] == nil {
				continue
			}
			if rollbackErr := a.rollbacks[i](); rollbackErr != nil {
				log.Warnf("rollback failed: %v", rollbackErr)
			}
		}
	}

	// Run cleanups in reverse order
	for i := len(a.cleanups) - 1; i >= 0; i-- {
		if a.cleanups[i] == nil {
			continue
		}
		a.cleanups[i]()
	}
}

// Setup sets up the APM injector
func (a *InjectorInstaller) Setup(ctx context.Context) error {
	var err error

	if err = setupAppArmor(ctx); err != nil {
		return err
	}

	// Create mandatory dirs
	err = os.MkdirAll("/var/log/datadog/dotnet", 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error creating /var/log/datadog/dotnet: %w", err)
	}
	// a umask 0022 is frequently set by default, so we need to change the permissions by hand
	err = os.Chmod("/var/log/datadog/dotnet", 0777)
	if err != nil {
		return fmt.Errorf("error changing permissions on /var/log/datadog/dotnet: %w", err)
	}
	err = os.Mkdir("/etc/datadog-agent/inject", 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error creating /etc/datadog-agent/inject: %w", err)
	}

	err = a.addLocalStableConfig(ctx)
	if err != nil {
		return fmt.Errorf("error adding stable config file: %w", err)
	}

	err = a.addInstrumentScripts(ctx)
	if err != nil {
		return fmt.Errorf("error adding install scripts: %w", err)
	}

	return a.Instrument(ctx)
}

// Remove removes the APM injector
func (a *InjectorInstaller) Remove(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_injector")
	defer func() { span.Finish(err) }()

	err = a.removeInstrumentScripts(ctx)
	if err != nil {
		return fmt.Errorf("error removing install scripts: %w", err)
	}

	err = removeAppArmor(ctx)
	if err != nil {
		return fmt.Errorf("error removing AppArmor profile: %w", err)
	}

	return a.Uninstrument(ctx)
}

// Instrument instruments the APM injector
func (a *InjectorInstaller) Instrument(ctx context.Context) error {
	if shouldInstrumentHost(a.Env) {
		if err := a.instrumentHost(ctx); err != nil {
			return err
		}
	}

	dockerIsInstalled := isDockerInstalled(ctx)
	if mustInstrumentDocker(a.Env) && !dockerIsInstalled {
		return errors.New("DD_APM_INSTRUMENTATION_ENABLED is set to docker but docker is not installed")
	}
	if shouldInstrumentDocker(a.Env) && dockerIsInstalled {
		// Set up defaults for agent sockets -- requires an agent restart
		if err := a.configureSocketsEnv(ctx); err != nil {
			return err
		}
		a.cleanups = append(a.cleanups, a.dockerConfigInstrument.cleanup)
		rollbackDocker, err := a.instrumentDocker(ctx)
		if err != nil {
			return err
		}
		a.rollbacks = append(a.rollbacks, rollbackDocker)

		// Verify that the docker runtime is as expected
		if err := a.verifyDockerRuntime(ctx); err != nil {
			return err
		}
	}

	return nil
}

// instrumentHost writes the injector into /etc/ld.so.preload for host injection.
// On a systemd host it also sets up the datadog-apm-inject unit that re-asserts
// the entry on every boot; the direct write always runs so the current boot is
// covered even when the unit was skipped.
func (a *InjectorInstaller) instrumentHost(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_host")
	defer func() { span.Finish(err) }()

	systemdRunning, _ := systemd.IsRunning()
	span.SetTag("systemd_running", systemdRunning)
	via := ViaPersistentPath
	if systemdRunning {
		// Best-effort: set up the systemd unit that re-asserts
		// /etc/ld.so.preload on every boot. This never fails the install — the
		// unit is a reliability enhancement, and the direct InstrumentLDPreload
		// below already persists across reboots on its own.
		if a.setupSystemdPreloadUnit(ctx, NewSystemdServiceManager()) {
			via = ViaTmpfsLink
		}
	}
	// Always write /etc/ld.so.preload directly so the current boot is covered
	// (and so host injection works even when the systemd unit was skipped).
	return a.InstrumentLDPreload(ctx, via)
}

// setupSystemdPreloadUnit installs (or refreshes) the datadog-apm-inject systemd unit
// that re-asserts /etc/ld.so.preload on every boot, when a datadog-installer
// supporting `apm instrument-start` is available. If none is available, or the
// unit setup or immediate start fails for any reason, it degrades to direct
// ld.so.preload management (the InstrumentLDPreload call in Instrument): the unit
// is a reliability enhancement and must never fail the package install. Any stale
// unit left by a previous install is removed so a doomed ExecStart is not left
// enabled.
//
// It returns true only when the unit was set up AND started successfully, so the
// caller may reference the launcher through the reboot-safe tmpfs symlink in its
// own direct ld.so.preload write. A false return — no supported installer, setup
// error, or the immediate start failed — means the caller must use the persistent
// path. On start failure the unit is removed: keeping a unit that cannot start
// enabled would be misleading and would not help, since the tmpfs symlink it
// creates is needed immediately (not only on next boot).
func (a *InjectorInstaller) setupSystemdPreloadUnit(ctx context.Context, mgr systemdServiceManager) (serviceRunning bool) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "setup_systemd_preload_unit")
	defer func() { span.Finish(nil) }()

	installerPath := mgr.InstallerPath()
	span.SetTag("installer_path", installerPath)

	if installerPath == "" {
		// No installer on disk supports `apm instrument-start` (no candidate at
		// all, or only older ones — e.g. the pinned agent in the DJM/Databricks
		// flow, or a stale `stable` symlink on upgrade). Skip the unit and rely on
		// the direct /etc/ld.so.preload write in Instrument, removing any stale
		// unit a previous install left behind.
		span.SetTag("mode", "direct_fallback")
		if mgr.ServiceFileExists() {
			if err := mgr.Uninstall(ctx); err != nil {
				log.Warnf("failed to remove stale apm-inject systemd service: %v", err)
			}
		}
		return false
	}

	span.SetTag("mode", "systemd")
	if err := mgr.Setup(ctx); err != nil {
		// Degrade rather than abort: clean up any partial unit and rely on the
		// direct /etc/ld.so.preload write in Instrument. The child
		// systemd_service_setup span records which step failed (failed_step).
		span.SetTag("mode", "direct_fallback_setup_error")
		span.SetTag("setup_error", err.Error())
		log.Warnf("failed to set up apm-inject systemd service, using direct /etc/ld.so.preload: %v", err)
		if mgr.ServiceFileExists() {
			if uErr := mgr.Uninstall(ctx); uErr != nil {
				log.Warnf("failed to clean up partial apm-inject systemd service: %v", uErr)
			}
		}
		return false
	}
	// The unit is installed, enabled, and running; roll it back on install failure.
	a.rollbacks = append(a.rollbacks, func() error {
		return mgr.Uninstall(ctx)
	})
	return true
}

// Uninstrument uninstruments the APM injector
func (a *InjectorInstaller) Uninstrument(ctx context.Context) error {
	errs := []error{}

	if shouldInstrumentHost(a.Env) {
		errs = append(errs, a.uninstrumentHost(ctx))
	}

	if shouldInstrumentDocker(a.Env) {
		errs = append(errs, a.uninstrumentDocker(ctx))
	}

	return multierr.Combine(errs...)
}

// uninstrumentHost removes host injection from /etc/ld.so.preload. If the
// datadog-apm-inject unit was installed, it is also uninstalled, and, as a
// safety net, the ld.so.preload entry is removed directly in case the unit's
// ExecStop did not run (e.g. it was in a failed state). The unit is uninstalled
// whenever its service file is present, regardless of what systemd.IsRunning
// reports — leaving an enabled stale unit behind would let a later boot
// re-add the ld.so.preload entry even though uninstrumentation reported
// success.
func (a *InjectorInstaller) uninstrumentHost(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_host")
	defer func() { span.Finish(err) }()

	systemdRunning, _ := systemd.IsRunning()
	span.SetTag("systemd_running", systemdRunning)

	mgr := NewSystemdServiceManager()
	if !mgr.ServiceFileExists() {
		return a.UninstrumentLDPreload(ctx)
	}
	return multierr.Combine(
		mgr.Uninstall(ctx),
		// Safety net: explicitly remove the ld.so.preload entry even if the
		// service's ExecStop did not run (e.g. service was in a failed state
		// when stopped). UninstrumentLDPreload is pure file I/O and idempotent.
		a.UninstrumentLDPreload(ctx),
	)
}

// setLDPreloadConfigContent sets the content of the LD preload configuration
func (a *InjectorInstaller) setLDPreloadConfigContent(ctx context.Context, ldSoPreload []byte) ([]byte, error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "set_ld_preload_config")
	defer func() { span.Finish(nil) }()

	launcherPreloadPath := a.ldPreloadEntry()

	// Replace the first known launcher entry with the active path in-place
	// (preserving its position in the file), then remove any additional known
	// entries. This handles all transition cases — including hosts left with two
	// entries by older code — without moving the entry to the end of the file.
	replaced := false
	staleRemoved := 0
	pattern := regexp.MustCompile(a.allKnownLauncherPattern())
	out := []byte(pattern.ReplaceAllStringFunc(string(ldSoPreload), func(_ string) string {
		if !replaced {
			replaced = true
			return launcherPreloadPath
		}
		staleRemoved++
		return ""
	}))
	if replaced {
		if staleRemoved > 0 {
			span.SetTag("action", "migrated_with_stale_removed")
			span.SetTag("stale_entries_removed", staleRemoved)
		} else if string(out) == string(ldSoPreload) {
			span.SetTag("action", "already_present")
		} else {
			span.SetTag("action", "migrated")
		}
		return out, nil
	}

	span.SetTag("action", "appended")
	var buf bytes.Buffer
	buf.Write(out)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(launcherPreloadPath)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// ldPreloadEntry returns the launcher path written to /etc/ld.so.preload. When
// resolved (systemd-managed hosts), this is the tmpfs symlink path; otherwise
// it falls back to the persistent launcher under installPath.
func (a *InjectorInstaller) ldPreloadEntry() string {
	if a.launcherPath != "" {
		return a.launcherPath
	}
	return path.Join(a.installPath, "inject", "launcher.preload.so")
}

// enableTmpfsLink (re)creates the tmpfs symlink pointing at the persistent
// injector payload and switches the ld.so.preload entry to the tmpfs path. It
// must only be called after the real launcher has been verified, so a broken
// launcher is never reachable through the symlink. On rollback the symlink is
// removed.
func (a *InjectorInstaller) enableTmpfsLink(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "enable_tmpfs_link")
	defer func() { span.Finish(err) }()
	target := path.Join(a.installPath, "inject")
	span.SetTag("target", target)
	span.SetTag("link", a.tmpfsInjectDir)
	// symlink.Set atomically replaces an existing link (temp symlink + rename),
	// so a concurrent reader never sees the link missing.
	if err := symlink.Set(a.tmpfsInjectDir, target); err != nil {
		return fmt.Errorf("failed to create tmpfs injector symlink %s -> %s: %w", a.tmpfsInjectDir, target, err)
	}
	a.launcherPath = path.Join(a.tmpfsInjectDir, "launcher.preload.so")
	a.rollbacks = append(a.rollbacks, func() error {
		return os.Remove(a.tmpfsInjectDir)
	})
	return nil
}

// allKnownLauncherPattern returns a regex that matches any known launcher path:
// the legacy deb path, the persistent OCI path (with or without a dynamic subdir),
// and the tmpfs symlink path. No whitespace anchors — use removeKnownLauncherEntries
// for whitespace-aware removal.
func (a *InjectorInstaller) allKnownLauncherPattern() string {
	alts := []string{
		regexp.QuoteMeta(oldLauncherPath),
		regexp.QuoteMeta(path.Join(a.installPath, "inject")) + launcherPatternSuffix,
	}
	if a.tmpfsInjectDir != "" {
		alts = append(alts, regexp.QuoteMeta(a.tmpfsInjectDir)+launcherPatternSuffix)
	}
	return "(" + strings.Join(alts, "|") + ")"
}

// removeKnownLauncherEntries strips every known launcher entry from ldSoPreload,
// including surrounding whitespace. Used by deleteLDPreloadConfigContent.
func (a *InjectorInstaller) removeKnownLauncherEntries(ldSoPreload []byte) []byte {
	p := a.allKnownLauncherPattern()
	matcher := regexp.MustCompile("^" + p + "(\\s*)|(\\s*)" + p)
	return []byte(matcher.ReplaceAllString(string(ldSoPreload), ""))
}

// deleteLDPreloadConfigContent removes all launcher entries from /etc/ld.so.preload.
func (a *InjectorInstaller) deleteLDPreloadConfigContent(_ context.Context, ldSoPreload []byte) ([]byte, error) {
	return a.removeKnownLauncherEntries(ldSoPreload), nil
}

func (a *InjectorInstaller) verifySharedLib(ctx context.Context, libPath string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "verify_shared_lib")
	defer func() { span.Finish(err) }()

	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		return fmt.Errorf("launcher library not found at %s", libPath)
	}

	echoPath, err := exec.LookPath("echo")
	if err != nil {
		// If echo is not found, to not block install,
		// we skip the test and add it to the span.
		span.SetTag("skipped", true)
		return nil
	}
	cmd := exec.Command(echoPath, "1")
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+libPath)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to verify injected lib %s (%w): %s", libPath, err, buf.String())
	}
	return nil
}

// LDPreloadVia selects how InstrumentLDPreload references the injector launcher
// in /etc/ld.so.preload.
type LDPreloadVia bool

const (
	// ViaPersistentPath writes the launcher's persistent OCI path directly. It
	// survives reboots without the systemd service and is the fallback when the
	// service is unavailable or not running.
	ViaPersistentPath LDPreloadVia = false
	// ViaTmpfsLink writes the reboot-safe /run tmpfs symlink path. Referencing the
	// launcher through a /run symlink that the datadog-apm-inject service recreates
	// each boot means a stale ld.so.preload entry becomes inert after a reboot, so
	// the host stays usable even if shutdown-time cleanup did not run. Only valid on
	// systemd-managed hosts where the service repopulates /run.
	ViaTmpfsLink LDPreloadVia = true
)

// InstrumentLDPreload directly adds the injector library to /etc/ld.so.preload.
// It is also called by the systemd service via "datadog-installer apm
// instrument-start host" (with ViaTmpfsLink) and must not attempt to manage
// systemd (it would loop).
func (a *InjectorInstaller) InstrumentLDPreload(ctx context.Context, via LDPreloadVia) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_ld_preload")
	defer func() { span.Finish(err) }()
	span.SetTag("use_tmpfs_link", bool(via))

	// Always verify the real launcher payload (under installPath) before
	// touching the symlink or ld.so.preload, so a broken launcher is never
	// activated — neither directly nor through the tmpfs symlink.
	ociLauncherPath := path.Join(a.installPath, "inject", "launcher.preload.so")
	log.Infof("Verifying APM injector launcher %s", ociLauncherPath)
	if err := a.verifySharedLib(ctx, ociLauncherPath); err != nil {
		// The launcher is broken. Strip any existing launcher entry (and the tmpfs
		// symlink) so we don't leave a dangling reference behind: a stale entry
		// pointing at a missing/broken .so makes ld.so print a "cannot be preloaded
		// ... ignored" warning on *every* process spawn. Cleaning is best-effort;
		// we still return the verification error so the caller (e.g. the systemd
		// ExecStart) fails and the unit is marked failed.
		if cleanErr := a.UninstrumentLDPreload(ctx); cleanErr != nil {
			log.Warnf("failed to clean %s after launcher verification failure: %v", ldSoPreloadPath, cleanErr)
		}
		return err
	}

	// On systemd-managed hosts, reference the launcher through the tmpfs
	// symlink instead of its persistent path (created only now that the
	// launcher has been verified good).
	if via == ViaTmpfsLink {
		if err := a.enableTmpfsLink(ctx); err != nil {
			return err
		}
	}

	launcherPath := a.ldPreloadEntry()
	span.SetTag("launcher_path", launcherPath)
	log.Infof("Adding APM injector launcher %s to %s", launcherPath, ldSoPreloadPath)
	a.cleanups = append(a.cleanups, a.ldPreloadFileInstrument.cleanup)
	rollback, err := a.ldPreloadFileInstrument.mutate(ctx)
	if err != nil {
		return err
	}
	a.rollbacks = append(a.rollbacks, rollback)
	log.Infof("APM injector launcher present in %s", ldSoPreloadPath)
	return nil
}

// UninstrumentLDPreloadTmpfs removes only the tmpfs launcher entry from
// /etc/ld.so.preload and the tmpfs symlink. Called by the datadog-apm-inject
// systemd service on stop (ExecStop). It intentionally leaves any persistent
// OCI entry untouched, making it safe to call on non-systemd hosts (where
// there is no tmpfs entry) and preventing it from accidentally removing
// persistent instrumentation.
func (a *InjectorInstaller) UninstrumentLDPreloadTmpfs(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_ld_preload_tmpfs")
	defer func() { span.Finish(err) }()
	if a.tmpfsInjectDir != "" {
		tmpfsPattern := regexp.QuoteMeta(a.tmpfsInjectDir) + launcherPatternSuffix
		matcher := regexp.MustCompile("^" + tmpfsPattern + "(\\s*)|(\\s*)" + tmpfsPattern)
		found := false
		mutator := newFileMutator(ldSoPreloadPath, func(_ context.Context, content []byte) ([]byte, error) {
			result := matcher.ReplaceAllString(string(content), "")
			found = result != string(content)
			return []byte(result), nil
		}, nil, nil)
		if _, err := mutator.mutate(ctx); err != nil {
			return err
		}
		span.SetTag("tmpfs_entry_found", found)
		if rmErr := os.Remove(a.tmpfsInjectDir); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warnf("failed to remove tmpfs injector symlink %s: %v", a.tmpfsInjectDir, rmErr)
		}
	}
	return nil
}

// UninstrumentLDPreload removes all launcher entries from /etc/ld.so.preload
// and the tmpfs symlink. Used by the full uninstrument flow (apm uninstrument).
func (a *InjectorInstaller) UninstrumentLDPreload(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_ld_preload")
	defer func() { span.Finish(err) }()
	log.Infof("Removing APM injector launcher from %s", ldSoPreloadPath)
	if _, err = a.ldPreloadFileUninstrument.mutate(ctx); err != nil {
		return err
	}
	if a.tmpfsInjectDir != "" {
		if rmErr := os.Remove(a.tmpfsInjectDir); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warnf("failed to remove tmpfs injector symlink %s: %v", a.tmpfsInjectDir, rmErr)
		}
	}
	log.Infof("APM injector launcher removed from %s", ldSoPreloadPath)
	return nil
}

// addInstrumentScripts writes the instrument scripts that come with the APM injector
// and override the previous instrument scripts if they exist
// These scripts are either:
// - Referenced in our public documentation, so we override them to use installer commands for consistency
// - Used on deb/rpm removal and may break the OCI in the process
func (a *InjectorInstaller) addInstrumentScripts(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "add_instrument_scripts")
	defer func() { span.Finish(err) }()

	hostMutator := newFileMutator(
		"/usr/bin/dd-host-install",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return embedded.ScriptDDHostInstall, nil
		},
		nil, nil,
	)
	a.cleanups = append(a.cleanups, hostMutator.cleanup)
	rollbackHost, err := hostMutator.mutate(ctx)
	if err != nil {
		return fmt.Errorf("failed to override dd-host-install: %w", err)
	}
	a.rollbacks = append(a.rollbacks, rollbackHost)
	err = os.Chmod("/usr/bin/dd-host-install", 0755)
	if err != nil {
		return fmt.Errorf("failed to change permissions of dd-host-install: %w", err)
	}

	containerMutator := newFileMutator(
		"/usr/bin/dd-container-install",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return embedded.ScriptDDContainerInstall, nil
		},
		nil, nil,
	)
	a.cleanups = append(a.cleanups, containerMutator.cleanup)
	rollbackContainer, err := containerMutator.mutate(ctx)
	if err != nil {
		return fmt.Errorf("failed to override dd-host-install: %w", err)
	}
	a.rollbacks = append(a.rollbacks, rollbackContainer)
	err = os.Chmod("/usr/bin/dd-container-install", 0755)
	if err != nil {
		return fmt.Errorf("failed to change permissions of dd-container-install: %w", err)
	}

	// Only override dd-cleanup if it exists
	_, err = os.Stat("/usr/bin/dd-cleanup")
	if err == nil {
		cleanupMutator := newFileMutator(
			"/usr/bin/dd-cleanup",
			func(_ context.Context, _ []byte) ([]byte, error) {
				return embedded.ScriptDDCleanup, nil
			},
			nil, nil,
		)
		a.cleanups = append(a.cleanups, cleanupMutator.cleanup)
		rollbackCleanup, err := cleanupMutator.mutate(ctx)
		if err != nil {
			return fmt.Errorf("failed to override dd-cleanup: %w", err)
		}
		a.rollbacks = append(a.rollbacks, rollbackCleanup)
		err = os.Chmod("/usr/bin/dd-cleanup", 0755)
		if err != nil {
			return fmt.Errorf("failed to change permissions of dd-cleanup: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if dd-cleanup exists on disk: %w", err)
	}
	return nil
}

// removeInstrumentScripts removes the install scripts that come with the APM injector
// if and only if they've been installed by the installer and not modified
func (a *InjectorInstaller) removeInstrumentScripts(ctx context.Context) (retErr error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_instrument_scripts")
	defer func() { span.Finish(retErr) }()

	for _, script := range []string{"dd-host-install", "dd-container-install", "dd-cleanup"} {
		path := filepath.Join("/usr/bin", script)
		_, err := os.Stat(path)
		if err == nil {
			err = os.Remove(path)
			if err != nil {
				return fmt.Errorf("failed to remove %s: %w", path, err)
			}
		}
	}
	return nil
}

func (a *InjectorInstaller) addLocalStableConfig(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "add_local_stable_config")
	defer func() { span.Finish(err) }()

	appMonitoringConfigMutator := newFileMutator(
		localStableConfigPath,
		func(_ context.Context, existing []byte) ([]byte, error) {
			cfg := config.ApplicationMonitoringConfig{
				Default: config.APMConfigurationDefault{},
			}
			hasChanged := false

			if len(existing) > 0 {
				err := yaml.Unmarshal(existing, &cfg)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal existing application_monitoring.yaml: %w", err)
				}
			}

			if a.Env.InstallScript.RuntimeMetricsEnabled != nil {
				hasChanged = true
				cfg.Default.RuntimeMetricsEnabled = a.Env.InstallScript.RuntimeMetricsEnabled
			}
			if a.Env.InstallScript.LogsInjection != nil {
				hasChanged = true
				cfg.Default.LogsInjection = a.Env.InstallScript.LogsInjection
			}
			if a.Env.InstallScript.APMTracingEnabled != nil {
				hasChanged = true
				cfg.Default.APMTracingEnabled = a.Env.InstallScript.APMTracingEnabled
			}
			if a.Env.InstallScript.DataStreamsEnabled != nil {
				hasChanged = true
				cfg.Default.DataStreamsEnabled = a.Env.InstallScript.DataStreamsEnabled
			}
			if a.Env.InstallScript.AppsecEnabled != nil {
				hasChanged = true
				cfg.Default.AppsecEnabled = a.Env.InstallScript.AppsecEnabled
			}
			if a.Env.InstallScript.IastEnabled != nil {
				hasChanged = true
				cfg.Default.IastEnabled = a.Env.InstallScript.IastEnabled
			}
			if a.Env.InstallScript.DataJobsEnabled != nil {
				hasChanged = true
				cfg.Default.DataJobsEnabled = a.Env.InstallScript.DataJobsEnabled
			}
			if a.Env.InstallScript.AppsecScaEnabled != nil {
				hasChanged = true
				cfg.Default.AppsecScaEnabled = a.Env.InstallScript.AppsecScaEnabled
			}
			if a.Env.InstallScript.ProfilingEnabled != "" {
				hasChanged = true
				cfg.Default.ProfilingEnabled = &a.Env.InstallScript.ProfilingEnabled
			}
			if a.Env.InstallScript.TracerLogsCollectionEnabled != nil {
				hasChanged = true
				cfg.Default.LogsCollectionEnabled = a.Env.InstallScript.TracerLogsCollectionEnabled
			}
			if a.Env.InstallScript.RumEnabled != nil {
				hasChanged = true
				cfg.Default.RumEnabled = a.Env.InstallScript.RumEnabled
			}
			if a.Env.InstallScript.RumApplicationID != "" {
				hasChanged = true
				cfg.Default.RumApplicationID = a.Env.InstallScript.RumApplicationID
			}
			if a.Env.InstallScript.RumClientToken != "" {
				hasChanged = true
				cfg.Default.RumClientToken = a.Env.InstallScript.RumClientToken
			}
			if a.Env.InstallScript.RumRemoteConfigurationID != "" {
				hasChanged = true
				cfg.Default.RumRemoteConfigurationID = a.Env.InstallScript.RumRemoteConfigurationID
			}
			if a.Env.InstallScript.RumSite != "" {
				hasChanged = true
				cfg.Default.RumSite = a.Env.InstallScript.RumSite
			}

			// Avoid creating a .backup file and overwriting the existing file if no changes were made
			if hasChanged {
				return yaml.Marshal(cfg)
			}
			return existing, nil
		},
		nil, nil,
	)
	rollback, err := appMonitoringConfigMutator.mutate(ctx)
	if err != nil {
		return err
	}
	err = os.Chmod(localStableConfigPath, 0644)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to set permissions for application_monitoring.yaml: %w", err)
	}

	a.rollbacks = append(a.rollbacks, rollback)
	return nil
}

func shouldInstrumentHost(execEnvs *env.Env) bool {
	switch execEnvs.InstallScript.APMInstrumentationEnabled {
	case env.APMInstrumentationEnabledHost, env.APMInstrumentationEnabledAll, env.APMInstrumentationNotSet:
		return true
	case env.APMInstrumentationEnabledDocker:
		return false
	default:
		log.Warnf("Unknown value for DD_APM_INSTRUMENTATION_ENABLED: %s. Supported values are all/docker/host", execEnvs.InstallScript.APMInstrumentationEnabled)
		return false
	}
}

func shouldInstrumentDocker(execEnvs *env.Env) bool {
	switch execEnvs.InstallScript.APMInstrumentationEnabled {
	case env.APMInstrumentationEnabledDocker, env.APMInstrumentationEnabledAll, env.APMInstrumentationNotSet:
		return true
	case env.APMInstrumentationEnabledHost:
		return false
	default:
		log.Warnf("Unknown value for DD_APM_INSTRUMENTATION_ENABLED: %s. Supported values are all/docker/host", execEnvs.InstallScript.APMInstrumentationEnabled)
		return false
	}
}

func mustInstrumentDocker(execEnvs *env.Env) bool {
	return execEnvs.InstallScript.APMInstrumentationEnabled == env.APMInstrumentationEnabledDocker
}
