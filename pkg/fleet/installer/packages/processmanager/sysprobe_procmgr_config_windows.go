// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var sysprobeInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "system-probe",
	binaryRelPath:     "bin/agent/system-probe.exe",
	configFileName:    "datadog-agent-sysprobe.yaml",
	embeddedConfig:    embedded.SysprobeWindowsProcmgrConfig,
	placeholderPrefix: "SYSPROBE",
}

// WriteSysprobeProcmgrConfig writes datadog-agent-sysprobe.yaml under installRootResolved\processes.d
// so dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WriteSysprobeProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, sysprobeInstallRootProcmgrSpec)
}

// RemoveSysprobeProcmgrConfig removes the system-probe processes.d YAML from
// installRootResolved\processes.d.
func RemoveSysprobeProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, sysprobeInstallRootProcmgrSpec)
}
