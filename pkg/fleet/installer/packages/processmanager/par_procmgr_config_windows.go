// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var parInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "PAR",
	binaryRelPath:     "bin/agent/privateactionrunner.exe",
	configFileName:    "datadog-agent-action.yaml",
	embeddedConfig:    embedded.PARWindowsProcmgrConfig,
	placeholderPrefix: "PAR",
}

// WritePARProcmgrConfig writes datadog-agent-action.yaml under installRootResolved\processes.d so
// dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WritePARProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, parInstallRootProcmgrSpec)
}

// RemovePARProcmgrConfig removes the PAR processes.d YAML from installRootResolved\processes.d.
func RemovePARProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, parInstallRootProcmgrSpec)
}

var parExecutorInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "PAR executor",
	binaryRelPath:     "bin/agent/privateactionrunner.exe",
	configFileName:    "datadog-agent-action-executor.yaml",
	embeddedConfig:    embedded.PARExecutorWindowsProcmgrConfig,
	placeholderPrefix: "PAR",
}

// WritePARExecutorProcmgrConfig writes datadog-agent-action-executor.yaml under
// installRootResolved\processes.d so dd-procmgrd knows about the PAR on-demand executor.
func WritePARExecutorProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, parExecutorInstallRootProcmgrSpec)
}

// RemovePARExecutorProcmgrConfig removes the PAR executor processes.d YAML from
// installRootResolved\processes.d.
func RemovePARExecutorProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, parExecutorInstallRootProcmgrSpec)
}
