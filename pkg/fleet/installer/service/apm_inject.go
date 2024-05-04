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
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	injectorConfigPrefix = []byte("# BEGIN LD PRELOAD CONFIG")
	injectorConfigSuffix = []byte("# END LD PRELOAD CONFIG")
)

const (
	injectorConfigTemplate = `
apm_config:
  receiver_socket: %s
use_dogstatsd: true
dogstatsd_socket: %s
`
	datadogConfigPath = "/etc/datadog-agent/datadog.yaml"
	ldSoPreloadPath   = "/etc/ld.so.preload"
	injectorPath      = "/opt/datadog-packages/datadog-apm-inject/stable"
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
func RemoveAPMInjector(ctx context.Context) {
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_injector")
	defer span.Finish()
	installer := newAPMInjectorInstaller(injectorPath)
	installer.Remove(ctx)
}

func newAPMInjectorInstaller(path string) *apmInjectorInstaller {
	a := &apmInjectorInstaller{
		installPath: path,
	}
	a.ldPreloadFileInstrument = newFileMutator(ldSoPreloadPath, a.setLDPreloadConfigContent, nil, nil)
	a.ldPreloadFileUninstrument = newFileMutator(ldSoPreloadPath, a.deleteLDPreloadConfigContent, nil, nil)
	a.agentConfigSockets = newFileMutator(datadogConfigPath, a.setAgentConfigContent, nil, nil)
	a.agentRemoveConfigSockets = newFileMutator(datadogConfigPath, a.deleteAgentConfigContent, nil, nil)
	a.dockerConfigInstrument = newFileMutator(dockerDaemonPath, a.setDockerConfigContent, nil, nil)
	a.dockerConfigUninstrument = newFileMutator(dockerDaemonPath, a.deleteDockerConfigContent, nil, nil)
	return a
}

type apmInjectorInstaller struct {
	installPath               string
	ldPreloadFileInstrument   *fileMutator
	ldPreloadFileUninstrument *fileMutator
	agentConfigSockets        *fileMutator
	agentRemoveConfigSockets  *fileMutator
	dockerConfigInstrument    *fileMutator
	dockerConfigUninstrument  *fileMutator
}

// Setup sets up the APM injector
func (a *apmInjectorInstaller) Setup(ctx context.Context) error {
	var err error
	var rollbackAgentConfig, rollbackLDPreload, rollbackDockerConfig func() error
	defer func() {
		if err != nil {
			// todo propagate rollbacks until success of package installation
			if rollbackLDPreload != nil {
				if err := rollbackLDPreload(); err != nil {
					log.Warnf("Failed to rollback ld preload: %v", err)
				}
			}
			if rollbackAgentConfig != nil {
				if err := rollbackAgentConfig(); err != nil {
					log.Warnf("Failed to rollback agent config: %v", err)
				}
			}
			if rollbackDockerConfig != nil {
				if err := rollbackDockerConfig(); err != nil {
					log.Warnf("Failed to rollback docker config: %v", err)
				}
			}
		}
	}()
	rollbackAgentConfig, err = a.agentConfigSockets.mutate()
	if err != nil {
		return err
	}
	if err := a.setRunPermissions(); err != nil {
		return err
	}
	rollbackLDPreload, err = a.ldPreloadFileInstrument.mutate()
	if err != nil {
		return err
	}

	// TODO only instrument docker if DD_APM_INSTRUMENTATION_ENABLED=docker
	// is set
	rollbackDockerConfig, err = a.setupDocker(ctx)
	return err
}

func (a *apmInjectorInstaller) Remove(ctx context.Context) {
	if _, err := a.agentRemoveConfigSockets.mutate(); err != nil {
		log.Warnf("Failed to remove agent config: %v", err)
	}
	if _, err := a.ldPreloadFileUninstrument.mutate(); err != nil {
		log.Warnf("Failed to remove ld preload config: %v", err)
	}
	// TODO docker only on DD_APM_INSTRUMENTATION_ENABLED=docker
	if err := a.uninstallDocker(ctx); err != nil {
		log.Warnf("Failed to remove docker config: %v", err)
	}
}

func (a *apmInjectorInstaller) setRunPermissions() error {
	return os.Chmod(path.Join(a.installPath, "inject", "run"), 0777)
}

// setLDPreloadConfigContent sets the content of the LD preload configuration
func (a *apmInjectorInstaller) setLDPreloadConfigContent(ldSoPreload []byte) ([]byte, error) {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	if strings.Contains(string(ldSoPreload), launcherPreloadPath) {
		// If the line of interest is already in /etc/ld.so.preload, return fast
		return ldSoPreload, nil
	}

	// Append the launcher preload path to the file
	if len(ldSoPreload) > 0 && ldSoPreload[len(ldSoPreload)-1] != '\n' {
		ldSoPreload = append(ldSoPreload, '\n')
	}
	ldSoPreload = append(ldSoPreload, []byte(launcherPreloadPath+"\n")...)
	return ldSoPreload, nil
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

// setAgentConfigContent adds the agent configuration for the APM injector if it is not there already
// We assume that the agent file has been created by the installer's postinst script
//
// Note: This is not safe, as it assumes there were no changes to the agent configuration made without
// restart by the user. This means that the agent can crash on restart. This is a limitation of the current
// installer system and this will be replaced by a proper experiment when available. This is a temporary
// solution to allow the APM injector to be installed, and if the agent crashes, we try to detect it and
// restore the previous configuration
func (a *apmInjectorInstaller) setAgentConfigContent(content []byte) ([]byte, error) {
	runPath := path.Join(a.installPath, "inject", "run")
	apmSocketPath := path.Join(runPath, "apm.socket")
	dsdSocketPath := path.Join(runPath, "dsd.socket")

	if !bytes.Contains(content, injectorConfigPrefix) {
		content = append(content, []byte("\n")...)
		content = append(content, injectorConfigPrefix...)
		content = append(content, []byte(
			fmt.Sprintf(injectorConfigTemplate, apmSocketPath, dsdSocketPath),
		)...)
		content = append(content, injectorConfigSuffix...)
		content = append(content, []byte("\n")...)
	}
	return content, nil
}

// deleteAgentConfigContent deletes the agent configuration for the APM injector
func (a *apmInjectorInstaller) deleteAgentConfigContent(content []byte) ([]byte, error) {
	start := bytes.Index(content, injectorConfigPrefix)
	end := bytes.Index(content, injectorConfigSuffix) + len(injectorConfigSuffix)
	if start == -1 || end == -1 || start >= end {
		// Config not found
		return content, nil
	}

	return append(content[:start], content[end:]...), nil
}

// restartTraceAgent restarts the stable trace agent
func restartTraceAgent(ctx context.Context) error {
	if err := restartUnit(ctx, "datadog-agent-trace.service"); err != nil {
		return err
	}
	return nil
}
