// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service/embedded"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/multierr"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	injectorPath    = "/opt/datadog-packages/datadog-apm-inject/stable"
	ldSoPreloadPath = "/etc/ld.so.preload"
	oldLauncherPath = "/opt/datadog/apm/inject/launcher.preload.so"
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "setup_injector")
	defer func() { span.Finish(tracer.WithError(err)) }()
	installer := newAPMInjectorInstaller(injectorPath)
	defer func() { installer.Finish(err) }()
	return installer.Setup(ctx)
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_injector")
	defer func() { span.Finish(tracer.WithError(err)) }()
	installer := newAPMInjectorInstaller(injectorPath)
	defer func() { installer.Finish(err) }()
	return installer.Remove(ctx)
}

// InstrumentAPMInjector instruments the APM injector
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(tracer.WithError(err)) }()
	installer := newAPMInjectorInstaller(injectorPath)
	installer.envs.InstallScript.APMInstrumentationEnabled = method
	defer func() { installer.Finish(err) }()
	return installer.Instrument(ctx)
}

// UninstrumentAPMInjector uninstruments the APM injector
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(tracer.WithError(err)) }()
	installer := newAPMInjectorInstaller(injectorPath)
	installer.envs.InstallScript.APMInstrumentationEnabled = method
	defer func() { installer.Finish(err) }()
	return installer.Uninstrument(ctx)
}

func newAPMInjectorInstaller(path string) *apmInjectorInstaller {
	a := &apmInjectorInstaller{
		installPath: path,
		envs:        env.FromEnv(),
	}
	a.ldPreloadFileInstrument = newFileMutator(ldSoPreloadPath, a.setLDPreloadConfigContent, nil, nil)
	a.ldPreloadFileUninstrument = newFileMutator(ldSoPreloadPath, a.deleteLDPreloadConfigContent, nil, nil)
	a.dockerConfigInstrument = newFileMutator(dockerDaemonPath, a.setDockerConfigContent, nil, nil)
	a.dockerConfigUninstrument = newFileMutator(dockerDaemonPath, a.deleteDockerConfigContent, nil, nil)
	return a
}

type apmInjectorInstaller struct {
	installPath               string
	ldPreloadFileInstrument   *fileMutator
	ldPreloadFileUninstrument *fileMutator
	dockerConfigInstrument    *fileMutator
	dockerConfigUninstrument  *fileMutator
	envs                      *env.Env

	rollbacks []func() error
	cleanups  []func()
}

// Finish cleans up the APM injector
// Runs rollbacks if an error is passed and always runs cleanups
func (a *apmInjectorInstaller) Finish(err error) {
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
func (a *apmInjectorInstaller) Setup(ctx context.Context) error {
	var err error

	// Set up defaults for agent sockets
	if err := a.configureSocketsEnv(ctx); err != nil {
		return err
	}
	// Symlinks for sysvinit
	if err := os.Symlink(envFilePath, "/etc/default/datadog-agent-trace"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to symlink %s to /etc/default/datadog-agent-trace: %w", envFilePath, err)
	}
	if err := os.Symlink(envFilePath, "/etc/default/datadog-agent"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to symlink %s to /etc/default/datadog-agent: %w", envFilePath, err)
	}
	systemdRunning, err := isSystemdRunning()
	if err != nil {
		return fmt.Errorf("failed to check if systemd is running: %w", err)
	}
	if systemdRunning {
		if err := addSystemDEnvOverrides(ctx, agentUnit); err != nil {
			return err
		}
		if err := addSystemDEnvOverrides(ctx, agentExp); err != nil {
			return err
		}
		if err := addSystemDEnvOverrides(ctx, traceAgentUnit); err != nil {
			return err
		}
		if err := addSystemDEnvOverrides(ctx, traceAgentExp); err != nil {
			return err
		}
		if err := systemdReload(ctx); err != nil {
			return err
		}
	}
	if err := setupAppArmor(ctx); err != nil {
		return err
	}

	// Create mandatory dirs
	err = os.Mkdir("/var/log/datadog/dotnet", 0777)
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

	err = a.addInstrumentScripts(ctx)
	if err != nil {
		return fmt.Errorf("error adding install scripts: %w", err)
	}

	return a.Instrument(ctx)
}

func (a *apmInjectorInstaller) Remove(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_injector")
	defer func() { span.Finish(tracer.WithError(err)) }()

	err = a.removeInstrumentScripts(ctx)
	if err != nil {
		return fmt.Errorf("error removing install scripts: %w", err)
	}

	return a.Uninstrument(ctx)
}

// Instrument instruments the APM injector
func (a *apmInjectorInstaller) Instrument(ctx context.Context) (retErr error) {
	// Check if the shared library is working before any instrumentation
	if err := a.verifySharedLib(ctx, path.Join(a.installPath, "inject", "launcher.preload.so")); err != nil {
		return err
	}

	if shouldInstrumentHost(a.envs) {
		a.cleanups = append(a.cleanups, a.ldPreloadFileInstrument.cleanup)
		rollbackLDPreload, err := a.ldPreloadFileInstrument.mutate(ctx)
		if err != nil {
			return err
		}
		a.rollbacks = append(a.rollbacks, rollbackLDPreload)
	}

	dockerIsInstalled := isDockerInstalled(ctx)
	if mustInstrumentDocker(a.envs) && !dockerIsInstalled {
		return fmt.Errorf("DD_APM_INSTRUMENTATION_ENABLED is set to docker but docker is not installed")
	}
	if shouldInstrumentDocker(a.envs) && dockerIsInstalled {
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

// Uninstrument uninstruments the APM injector
func (a *apmInjectorInstaller) Uninstrument(ctx context.Context) error {
	errs := []error{}

	if shouldInstrumentHost(a.envs) {
		_, hostErr := a.ldPreloadFileUninstrument.mutate(ctx)
		errs = append(errs, hostErr)
	}

	if shouldInstrumentDocker(a.envs) {
		dockerErr := a.uninstrumentDocker(ctx)
		errs = append(errs, dockerErr)
	}
	appArmorErr := removeAppArmor(ctx)
	errs = append(errs, appArmorErr)

	return multierr.Combine(errs...)
}

// setLDPreloadConfigContent sets the content of the LD preload configuration
func (a *apmInjectorInstaller) setLDPreloadConfigContent(_ context.Context, ldSoPreload []byte) ([]byte, error) {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	if strings.Contains(string(ldSoPreload), launcherPreloadPath) {
		// If the line of interest is already in /etc/ld.so.preload, return fast
		return ldSoPreload, nil
	}

	if bytes.Contains(ldSoPreload, []byte(oldLauncherPath)) {
		return bytes.ReplaceAll(ldSoPreload, []byte(oldLauncherPath), []byte(launcherPreloadPath)), nil
	}

	var buf bytes.Buffer
	buf.Write(ldSoPreload)
	// Append the launcher preload path to the file
	if len(ldSoPreload) > 0 && ldSoPreload[len(ldSoPreload)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(launcherPreloadPath)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// deleteLDPreloadConfigContent deletes the content of the LD preload configuration
func (a *apmInjectorInstaller) deleteLDPreloadConfigContent(_ context.Context, ldSoPreload []byte) ([]byte, error) {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	if !strings.Contains(string(ldSoPreload), launcherPreloadPath) {
		// If the line of interest isn't there, return fast
		return ldSoPreload, nil
	}

	// Possible configurations of the preload path, order matters
	replacementsToTest := [][]byte{
		[]byte(launcherPreloadPath + "\n"),
		[]byte("\n" + launcherPreloadPath),
		[]byte(launcherPreloadPath + " "),
		[]byte(" " + launcherPreloadPath),
	}
	for _, replacement := range replacementsToTest {
		ldSoPreloadNew := bytes.Replace(ldSoPreload, replacement, []byte{}, 1)
		if !bytes.Equal(ldSoPreloadNew, ldSoPreload) {
			return ldSoPreloadNew, nil
		}
	}
	if bytes.Equal(ldSoPreload, []byte(launcherPreloadPath)) {
		// If the line is the only one in the file without newlines, return an empty file
		return []byte{}, nil
	}

	return nil, fmt.Errorf("failed to remove %s from %s", launcherPreloadPath, ldSoPreloadPath)
}

func (a *apmInjectorInstaller) verifySharedLib(ctx context.Context, libPath string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "verify_shared_lib")
	defer func() { span.Finish(tracer.WithError(err)) }()
	echoPath, err := exec.LookPath("echo")
	if err != nil {
		return fmt.Errorf("failed to find echo: %w", err)
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

// addInstrumentScripts writes the instrument scripts that come with the APM injector
// and override the previous instrument scripts if they exist
// These scripts are either:
// - Referenced in our public documentation, so we override them to use installer commands for consistency
// - Used on deb/rpm removal and may break the OCI in the process
func (a *apmInjectorInstaller) addInstrumentScripts(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "add_instrument_scripts")
	defer func() { span.Finish(tracer.WithError(err)) }()

	hostMutator := newFileMutator(
		"/usr/bin/dd-host-install",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return embedded.FS.ReadFile("dd-host-install")
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
			return embedded.FS.ReadFile("dd-container-install")
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
				return embedded.FS.ReadFile("dd-cleanup")
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
func (a *apmInjectorInstaller) removeInstrumentScripts(ctx context.Context) (retErr error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_instrument_scripts")
	defer func() { span.Finish(tracer.WithError(retErr)) }()

	for _, script := range []string{"dd-host-install", "dd-container-install", "dd-cleanup"} {
		path := filepath.Join("/usr/bin", script)
		_, err := os.Stat(path)
		if err == nil {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", path, err)
			}
			embeddedContent, err := embedded.FS.ReadFile(script)
			if err != nil {
				return fmt.Errorf("failed to read embedded %s: %w", script, err)
			}
			if bytes.Equal(content, embeddedContent) {
				err = os.Remove(path)
				if err != nil {
					return fmt.Errorf("failed to remove %s: %w", path, err)
				}
			}
		}
	}
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
