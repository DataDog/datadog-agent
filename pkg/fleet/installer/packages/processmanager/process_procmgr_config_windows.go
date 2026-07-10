// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var processInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "process-agent",
	binaryRelPath:     "bin/agent/process-agent.exe",
	configFileName:    "datadog-agent-process.yaml",
	embeddedConfig:    embedded.ProcessWindowsProcmgrConfig,
	placeholderPrefix: "PROCESS",
}

// WriteProcessProcmgrConfig writes datadog-agent-process.yaml under installRootResolved\processes.d so
// dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WriteProcessProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, processInstallRootProcmgrSpec)
}

// RemoveProcessProcmgrConfig removes the process-agent processes.d YAML from installRootResolved\processes.d.
func RemoveProcessProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, processInstallRootProcmgrSpec)
}
