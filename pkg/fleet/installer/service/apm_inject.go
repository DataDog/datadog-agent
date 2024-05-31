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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	injectorPath    = "/opt/datadog-packages/datadog-apm-inject/stable"
	ldSoPreloadPath = "/etc/ld.so.preload"
	oldLauncherPath = "/opt/datadog/apm/inject/launcher.preload.so"
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector(ctx context.Context) error {
	var err error
	span, ctx := tracer.StartSpanFromContext(ctx, "setup_injector")
	defer span.Finish(tracer.WithError(err))
	installer := newAPMInjectorInstaller(injectorPath)
	return installer.Setup(ctx)
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector(ctx context.Context) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_injector")
	defer span.Finish()
	installer := newAPMInjectorInstaller(injectorPath)
	return installer.Remove(ctx)
}

func newAPMInjectorInstaller(path string) *apmInjectorInstaller {
	a := &apmInjectorInstaller{
		installPath: path,
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
}

// Setup sets up the APM injector
func (a *apmInjectorInstaller) Setup(ctx context.Context) (err error) {
	// /var/log/datadog is created by default with datadog-installer install
	err = os.Mkdir("/var/log/datadog/dotnet", 0777)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error creating /var/log/datadog/dotnet: %w", err)
	}
	// a umask 0022 is frequently set by default, so we need to change the permissions by hand
	err = os.Chmod("/var/log/datadog/dotnet", 0777)
	if err != nil {
		return fmt.Errorf("error changing permissions on /var/log/datadog/dotnet: %w", err)
	}
	// Check if the shared library is working before adding it to the preload
	if err := a.verifySharedLib(path.Join(a.installPath, "inject", "launcher.preload.so")); err != nil {
		return err
	}

	rollbackLDPreload, err := a.ldPreloadFileInstrument.mutate()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := rollbackLDPreload(); err != nil {
				log.Warnf("Failed to rollback agent config: %v", err)
			}
		}
	}()

	// TODO only instrument docker if DD_APM_INSTRUMENTATION_ENABLED=docker
	rollbackDockerConfig, err := a.setupDocker(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := rollbackDockerConfig(); err != nil {
				log.Warnf("Failed to rollback agent config: %v", err)
			}
		}
	}()

	// Verify that the docker runtime is as expected
	if err := a.verifyDockerRuntime(); err != nil {
		return err
	}

	// Set up defaults for agent sockets
	if err = configureSocketsEnv(); err != nil {
		return
	}
	if err = addSystemDEnvOverrides(agentUnit); err != nil {
		return
	}
	if err = addSystemDEnvOverrides(agentExp); err != nil {
		return
	}
	if err = addSystemDEnvOverrides(traceAgentUnit); err != nil {
		return
	}
	if err = addSystemDEnvOverrides(traceAgentExp); err != nil {
		return
	}

	return nil
}

func (a *apmInjectorInstaller) Remove(ctx context.Context) error {
	if _, err := a.ldPreloadFileUninstrument.mutate(); err != nil {
		log.Warnf("Failed to remove ld preload config: %v", err)
	}
	// TODO docker only on DD_APM_INSTRUMENTATION_ENABLED=docker
	if err := a.uninstallDocker(ctx); err != nil {
		log.Warnf("Failed to remove docker config: %v", err)
	}
	// TODO: return error to caller?
	return nil
}

// setLDPreloadConfigContent sets the content of the LD preload configuration
func (a *apmInjectorInstaller) setLDPreloadConfigContent(ldSoPreload []byte) ([]byte, error) {
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
func (a *apmInjectorInstaller) deleteLDPreloadConfigContent(ldSoPreload []byte) ([]byte, error) {
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

func (a *apmInjectorInstaller) verifySharedLib(libPath string) error {
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
