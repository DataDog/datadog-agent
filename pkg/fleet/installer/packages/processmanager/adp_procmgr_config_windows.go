// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var adpInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "ADP",
	binaryRelPath:     "bin/agent/agent-data-plane.exe",
	configFileName:    "datadog-agent-data-plane.yaml",
	embeddedConfig:    embedded.ADPWindowsProcmgrConfig,
	placeholderPrefix: "ADP",
}

// WriteADPProcmgrConfig writes datadog-agent-data-plane.yaml under installRootResolved\processes.d so
// dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WriteADPProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, adpInstallRootProcmgrSpec)
}

// RemoveADPProcmgrConfig removes the ADP processes.d YAML from installRootResolved\processes.d.
func RemoveADPProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, adpInstallRootProcmgrSpec)
}
