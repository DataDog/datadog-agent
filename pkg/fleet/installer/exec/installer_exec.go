// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// InstallerExec is an implementation of the Installer interface that uses the installer binary.
type InstallerExec struct {
	env              *env.Env
	installerBinPath string
}

// NewInstallerExec returns a new InstallerExec.
func NewInstallerExec(env *env.Env, installerBinPath string) *InstallerExec {
	return &InstallerExec{
		env:              env,
		installerBinPath: installerBinPath,
	}
}

type installerCmd struct {
	*exec.Cmd
	span *telemetry.Span
	ctx  context.Context
}

func (i *InstallerExec) newInstallerCmdCustomPathDetached(ctx context.Context, command string, path string, args ...string) *installerCmd {
	span, ctx := telemetry.StartSpanFromContext(ctx, "installer."+command)
	span.SetTag("args", strings.Join(args, " "))
	// NOTE: We very intentionally don't provide ctx to exec.Command.
	//       exec.Command will kill the process if the context is cancelled. We don't want that here since
	//       it is supposed to be a detached process that may live longer than the current process.
	cmd := exec.Command(path, append([]string{command}, args...)...)
	// We're running this process in the background, so we don't intend to collect any output from it.
	// We set channels to nil here because os/exec waits on these pipes to close even after
	// the process terminates which can cause us (or our parent) to be forever blocked
	// by this child process or any children it creates, which may inherit any of these handles
	// and keep them open.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return i.setupInstallerCmd(ctx, span, cmd)
}

func (i *InstallerExec) newInstallerCmdCustomPath(ctx context.Context, command string, path string, args ...string) *installerCmd {
	span, ctx := telemetry.StartSpanFromContext(ctx, "installer."+command)
	span.SetTag("args", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, path, append([]string{command}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return i.setupInstallerCmd(ctx, span, cmd)
}

func (i *InstallerExec) setupInstallerCmd(ctx context.Context, span *telemetry.Span, cmd *exec.Cmd) *installerCmd {
	env := i.env.ToEnv()
	env = append(os.Environ(), env...)
	env = append(env, telemetry.EnvFromContext(ctx)...)
	cmd.Env = env
	cmd = i.newInstallerCmdPlatform(cmd)
	return &installerCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

func (i *InstallerExec) newInstallerCmd(ctx context.Context, command string, args ...string) *installerCmd {
	return i.newInstallerCmdCustomPath(ctx, command, i.installerBinPath, args...)
}

func (i *InstallerExec) newInstallerCmdDetached(ctx context.Context, command string, args ...string) *installerCmd {
	return i.newInstallerCmdCustomPathDetached(ctx, command, i.installerBinPath, args...)
}

// Install installs a package.
func (i *InstallerExec) Install(ctx context.Context, url string, args []string) (err error) {
	var cmdLineArgs = []string{url}
	if len(args) > 0 {
		cmdLineArgs = append(cmdLineArgs, "--install_args", strings.Join(args, ","))
	}
	cmd := i.newInstallerCmd(ctx, "install", cmdLineArgs...)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// SetupInstaller runs the setup command.
func (i *InstallerExec) SetupInstaller(ctx context.Context, path string) (err error) {
	cmd := i.newInstallerCmd(ctx, "setup-installer", path)
	defer func() { cmd.span.Finish(nil) }()
	return cmd.Run()
}

// ForceInstall installs a package, even if it's already installed.
func (i *InstallerExec) ForceInstall(ctx context.Context, url string, args []string) (err error) {
	var cmdLineArgs = []string{url, "--force"}
	if len(args) > 0 {
		cmdLineArgs = append(cmdLineArgs, "--install_args", strings.Join(args, ","))
	}
	cmd := i.newInstallerCmd(ctx, "install", cmdLineArgs...)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// Remove removes a package.
func (i *InstallerExec) Remove(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove", pkg)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// Purge - noop, must be called by the package manager on uninstall.
func (i *InstallerExec) Purge(_ context.Context) {
	panic("don't call Purge directly")
}

// InstallExperiment installs an experiment.
func (i *InstallerExec) InstallExperiment(ctx context.Context, url string) (err error) {
	cmd := i.newInstallerCmd(ctx, "install-experiment", url)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// RemoveExperiment removes an experiment.
func (i *InstallerExec) RemoveExperiment(ctx context.Context, pkg string) (err error) {
	var cmd *installerCmd
	// on windows we need to make a copy of installer binary so that it isn't in use
	// while the MSI tries to remove it
	if runtime.GOOS == "windows" && pkg == "datadog-agent" {
		repositories := repository.NewRepositories(paths.PackagesPath, nil)
		tmpDir, err := repositories.MkdirTemp()
		if err != nil {
			return fmt.Errorf("error creating temp dir: %w", err)
		}
		// this might not get run as this processes will be killed during the stop
		defer os.RemoveAll(tmpDir)

		// copy our installerPath to temp location
		installerPath := filepath.Join(tmpDir, "datadog-installer.exe")
		err = paths.CopyFile(i.installerBinPath, installerPath)
		if err != nil {
			return fmt.Errorf("error copying installer binary: %w", err)
		}
		cmd = i.newInstallerCmdCustomPath(ctx, "remove-experiment", installerPath, pkg)
	} else {
		cmd = i.newInstallerCmd(ctx, "remove-experiment", pkg)
	}
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// PromoteExperiment promotes an experiment to stable.
func (i *InstallerExec) PromoteExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "promote-experiment", pkg)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// InstallConfigExperiment installs an experiment.
func (i *InstallerExec) InstallConfigExperiment(
	ctx context.Context, pkg string, operations config.Operations, secrets map[string]string,
) (err error) {
	operationsBytes, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("error marshalling config operations: %w", err)
	}
	cmdLineArgs := []string{pkg, string(operationsBytes)}
	cmd := i.newInstallerCmd(ctx, "install-config-experiment", cmdLineArgs...)

	if secrets == nil {
		secrets = make(map[string]string)
	}
	secretsBytes, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("error marshalling decrypted secrets: %w", err)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	defer func() { cmd.span.Finish(err) }()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// Write secrets to stdin
	if _, err := stdinPipe.Write(secretsBytes); err != nil {
		stdinPipe.Close()
		return fmt.Errorf("error writing secrets to stdin: %w", err)
	}
	stdinPipe.Close()
	return cmd.Wait()
}

// RemoveConfigExperiment removes an experiment.
func (i *InstallerExec) RemoveConfigExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove-config-experiment", pkg)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// PromoteConfigExperiment promotes an experiment to stable.
func (i *InstallerExec) PromoteConfigExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "promote-config-experiment", pkg)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// GarbageCollect runs the garbage collector.
func (i *InstallerExec) GarbageCollect(ctx context.Context) (err error) {
	cmd := i.newInstallerCmd(ctx, "garbage-collect")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// InstrumentAPMInjector instruments the APM auto-injector.
func (i *InstallerExec) InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	cmd := i.newInstallerCmd(ctx, "apm instrument", method)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// UninstrumentAPMInjector uninstruments the APM auto-injector.
func (i *InstallerExec) UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	cmd := i.newInstallerCmd(ctx, "apm uninstrument", method)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// InstallExtensions installs multiple extensions.
func (i *InstallerExec) InstallExtensions(ctx context.Context, url string, extensions []string) (err error) {
	cmdLineArgs := append([]string{url}, extensions...)
	cmd := i.newInstallerCmd(ctx, "extension install", cmdLineArgs...)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// RemoveExtensions removes multiple extensions.
func (i *InstallerExec) RemoveExtensions(ctx context.Context, pkg string, extensions []string) (err error) {
	cmdLineArgs := append([]string{pkg}, extensions...)
	cmd := i.newInstallerCmd(ctx, "extension remove", cmdLineArgs...)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// SaveExtensions saves the extensions to a specific location on disk.
func (i *InstallerExec) SaveExtensions(ctx context.Context, pkg string, path string) (err error) {
	cmd := i.newInstallerCmd(ctx, "extension save", pkg, path)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// RestoreExtensions restores the extensions from a specific location on disk.
func (i *InstallerExec) RestoreExtensions(ctx context.Context, url string, path string) (err error) {
	cmd := i.newInstallerCmd(ctx, "extension restore", url, path)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// IsInstalled checks if a package is installed.
func (i *InstallerExec) IsInstalled(ctx context.Context, pkg string) (_ bool, err error) {
	cmd := i.newInstallerCmd(ctx, "is-installed", pkg)
	defer func() { cmd.span.Finish(err) }()
	err = cmd.Run()
	if err != nil && cmd.ProcessState.ExitCode() == 10 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DefaultPackages returns the default packages to install.
func (i *InstallerExec) DefaultPackages(ctx context.Context) (_ []string, err error) {
	cmd := i.newInstallerCmd(ctx, "default-packages")
	defer func() { cmd.span.Finish(err) }()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error running default-packages: %w\n%s", err, stderr.String())
	}
	var defaultPackages []string
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if line == "" {
			continue
		}
		defaultPackages = append(defaultPackages, line)
	}
	return defaultPackages, nil
}

// Setup runs the setup command.
func (i *InstallerExec) Setup(ctx context.Context) (err error) {
	cmd := i.newInstallerCmd(ctx, "setup")
	defer func() { cmd.span.Finish(err) }()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error running setup: %w\n%s", err, stderr.String())
	}
	return nil
}

// AvailableDiskSpace returns the available disk space.
func (i *InstallerExec) AvailableDiskSpace() (uint64, error) {
	repositories := repository.NewRepositories(paths.PackagesPath, nil)
	return repositories.AvailableDiskSpace()
}

// getStates retrieves the state of all packages & their configuration from disk.
func (i *InstallerExec) getStates(ctx context.Context) (repo *repository.PackageStates, err error) {
	cmd := i.newInstallerCmd(ctx, "get-states")
	defer func() { cmd.span.Finish(err) }()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting state from disk: %w\n%s", err, stderr.String())
	}
	var pkgStates *repository.PackageStates
	err = json.Unmarshal(stdout.Bytes(), &pkgStates)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling state from disk: %w\n`%s`", err, stdout.String())
	}

	return pkgStates, nil
}

// State returns the state of a package.
func (i *InstallerExec) State(ctx context.Context, pkg string) (repository.State, error) {
	allStates, err := i.ConfigAndPackageStates(ctx)
	if err != nil {
		return repository.State{}, err
	}
	return allStates.States[pkg], nil
}

// ConfigState returns the state of a package's configuration.
func (i *InstallerExec) ConfigState(ctx context.Context, pkg string) (repository.State, error) {
	allStates, err := i.ConfigAndPackageStates(ctx)
	if err != nil {
		return repository.State{}, err
	}
	return allStates.ConfigStates[pkg], nil
}

// ConfigAndPackageStates returns the states of all packages' configurations and packages.
func (i *InstallerExec) ConfigAndPackageStates(ctx context.Context) (*repository.PackageStates, error) {
	allStates, err := i.getStates(ctx)
	if err != nil {
		return nil, err
	}
	return allStates, nil
}

// Close cleans up any resources.
func (i *InstallerExec) Close() error {
	return nil
}

func (iCmd *installerCmd) Run() error {
	var errBuf bytes.Buffer
	iCmd.Stderr = &errBuf
	err := iCmd.Cmd.Run()
	if err == nil {
		return nil
	}

	if len(errBuf.Bytes()) == 0 {
		return fmt.Errorf("run failed: %w", err)
	}

	installerError := installerErrors.FromJSON(strings.TrimSpace(errBuf.String()))
	return fmt.Errorf("run failed: %w \n%s", installerError, err.Error())
}

// RunHook runs a hook for a given package.
func (i *InstallerExec) RunHook(ctx context.Context, hookContext string) (err error) {
	cmd := i.newInstallerCmd(ctx, "hooks", hookContext)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// StartPackageCommandDetached starts a package-specific command for a given package in the background with detached standard IO.
func (i *InstallerExec) StartPackageCommandDetached(ctx context.Context, packageName string, command string) (err error) {
	cmd := i.newInstallerCmdDetached(ctx, "package-command", packageName, command)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Start()
}
