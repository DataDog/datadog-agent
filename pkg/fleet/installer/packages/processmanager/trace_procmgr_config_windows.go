// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var traceInstallRootProcmgrSpec = installRootProcmgrSpec{
	logLabel:          "trace-agent",
	binaryRelPath:     "bin/agent/trace-agent.exe",
	configFileName:    "datadog-agent-trace.yaml",
	embeddedConfig:    embedded.TraceWindowsProcmgrConfig,
	placeholderPrefix: "TRACE",
}

// WriteTraceProcmgrConfig writes datadog-agent-trace.yaml under installRootResolved\processes.d
// so dd-procmgrd picks it up. installRootResolved is the resolved MSI Program Files install root.
func WriteTraceProcmgrConfig(installRootResolved string) error {
	return writeInstallRootProcmgrConfig(installRootResolved, traceInstallRootProcmgrSpec)
}

// RemoveTraceProcmgrConfig removes the trace-agent processes.d YAML from
// installRootResolved\processes.d.
func RemoveTraceProcmgrConfig(installRootResolved string) error {
	return removeInstallRootProcmgrConfig(installRootResolved, traceInstallRootProcmgrSpec)
}
