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

// GetSystemdUnit returns the systemd unit for the given name.
func GetSystemdUnit(name string) []byte {
	return units[name]
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
	units = map[string][]byte{
		"datadog-agent.service":               mustReadSystemdUnit("datadog-agent.service", stableData),
		"datadog-agent-exp.service":           mustReadSystemdUnit("datadog-agent.service", expData),
		"datadog-agent-installer.service":     mustReadSystemdUnit("datadog-agent-installer.service", stableData),
		"datadog-agent-installer-exp.service": mustReadSystemdUnit("datadog-agent-installer.service", expData),
		"datadog-agent-trace.service":         mustReadSystemdUnit("datadog-agent-trace.service", stableData),
		"datadog-agent-trace-exp.service":     mustReadSystemdUnit("datadog-agent-trace.service", expData),
		"datadog-agent-process.service":       mustReadSystemdUnit("datadog-agent-process.service", stableData),
		"datadog-agent-process-exp.service":   mustReadSystemdUnit("datadog-agent-process.service", expData),
		"datadog-agent-security.service":      mustReadSystemdUnit("datadog-agent-security.service", stableData),
		"datadog-agent-security-exp.service":  mustReadSystemdUnit("datadog-agent-security.service", expData),
		"datadog-agent-sysprobe.service":      mustReadSystemdUnit("datadog-agent-sysprobe.service", stableData),
		"datadog-agent-sysprobe-exp.service":  mustReadSystemdUnit("datadog-agent-sysprobe.service", expData),
	}
)
