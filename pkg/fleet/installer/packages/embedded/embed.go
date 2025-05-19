// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package embedded provides embedded files for the installer.
package embedded

import (
	"bytes"
	"embed"
	"path/filepath"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

// FS is the embedded filesystem for the installer.
//
//go:embed *
var FS embed.FS

type systemdTemplateData struct {
	InstallDir       string
	EtcDir           string
	FleetPoliciesDir string
	Stable           bool
}

func mustReadSystemdUnit(name string, data systemdTemplateData) []byte {
	tmpl, err := template.ParseFS(FS, filepath.Join("systemd", name+".tmpl"))
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// SystemdUnits is a map of systemd units to their content.
type SystemdUnits struct {
	DatadogAgentService             []byte
	DatadogAgentExpService          []byte
	DatadogAgentInstallerService    []byte
	DatadogAgentInstallerExpService []byte
	DatadogAgentTraceService        []byte
	DatadogAgentTraceExpService     []byte
	DatadogAgentProcessService      []byte
	DatadogAgentProcessExpService   []byte
	DatadogAgentSecurityService     []byte
	DatadogAgentSecurityExpService  []byte
	DatadogAgentSysprobeService     []byte
	DatadogAgentSysprobeExpService  []byte
}

var (
	stableData = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/stable",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		Stable:           true,
	}
	expData = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/experiment",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/experiment",
		Stable:           false,
	}

	// Units is a map of systemd units to their content.
	Units = SystemdUnits{
		DatadogAgentService:             mustReadSystemdUnit("datadog-agent.service", stableData),
		DatadogAgentExpService:          mustReadSystemdUnit("datadog-agent.service", expData),
		DatadogAgentInstallerService:    mustReadSystemdUnit("datadog-agent-installer.service", stableData),
		DatadogAgentInstallerExpService: mustReadSystemdUnit("datadog-agent-installer.service", expData),
		DatadogAgentTraceService:        mustReadSystemdUnit("datadog-agent-trace.service", stableData),
		DatadogAgentTraceExpService:     mustReadSystemdUnit("datadog-agent-trace.service", expData),
		DatadogAgentProcessService:      mustReadSystemdUnit("datadog-agent-process.service", stableData),
		DatadogAgentProcessExpService:   mustReadSystemdUnit("datadog-agent-process.service", expData),
		DatadogAgentSecurityService:     mustReadSystemdUnit("datadog-agent-security.service", stableData),
		DatadogAgentSecurityExpService:  mustReadSystemdUnit("datadog-agent-security.service", expData),
		DatadogAgentSysprobeService:     mustReadSystemdUnit("datadog-agent-sysprobe.service", stableData),
		DatadogAgentSysprobeExpService:  mustReadSystemdUnit("datadog-agent-sysprobe.service", expData),
	}
)
