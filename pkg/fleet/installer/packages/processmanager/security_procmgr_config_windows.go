// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var securityInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "security-agent",
	binaryRelPath:     "bin/agent/security-agent.exe",
	configFileName:    "datadog-agent-security.yaml",
	embeddedConfig:    embedded.SecurityWindowsProcmgrConfig,
	placeholderPrefix: "SECURITY",
}

// WriteSecurityProcmgrConfig writes datadog-agent-security.yaml under installRootResolved\processes.d
// so dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WriteSecurityProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, securityInstallRootProcmgrSpec)
}

// RemoveSecurityProcmgrConfig removes the security-agent processes.d YAML from
// installRootResolved\processes.d.
func RemoveSecurityProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, securityInstallRootProcmgrSpec)
}
