// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package windowsunits is the registry of the Windows service units.
package windowsunits

import (
	"embed"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
)

//go:embed gen
var generated embed.FS

// Placeholders the generator renders in place of the install-time roots.
const (
	InstallRootPlaceholder = "__INSTALL_ROOT__"
	EtcRootPlaceholder     = "__ETC_ROOT__"
)

// Procmgr holds the processes.d entries. Binary is relative to the root the entry is rendered with.
var Procmgr = struct {
	DataPlane, Action, ActionExecutor, ParControl, Process, Sysprobe, Trace, Security, DDOT units.Unit
}{
	DataPlane:      procmgr("datadog-agent-data-plane", "bin/agent/agent-data-plane.exe"),
	Action:         procmgr("datadog-agent-action", "bin/agent/privateactionrunner.exe"),
	ActionExecutor: procmgr("datadog-agent-action-executor", "bin/agent/privateactionrunner.exe"),
	ParControl:     procmgr("datadog-agent-par-control", "bin/agent/par-control.exe"),
	Process:        procmgr("datadog-agent-process", "bin/agent/process-agent.exe"),
	Sysprobe:       procmgr("datadog-agent-sysprobe", "bin/agent/system-probe.exe"),
	Trace:          procmgr("datadog-agent-trace", "bin/agent/trace-agent.exe"),
	Security:       procmgr("datadog-agent-security", "bin/agent/security-agent.exe"),
	DDOT:           procmgr("datadog-agent-ddot", "ext/ddot/embedded/bin/otel-agent.exe"),
}

// SCM holds the Windows services, created by the MSI or the package hooks.
var SCM = struct {
	OTel, Procmgr, Trace, Process, Security, Sysprobe, Action, Installer, CoreAgent units.Unit
}{
	OTel:      scm("datadog-otel-agent"),
	Procmgr:   scm("dd-procmgr-service"),
	Trace:     scm("datadog-trace-agent"),
	Process:   scm("datadog-process-agent"),
	Security:  scm("datadog-security-agent"),
	Sysprobe:  scm("datadog-system-probe"),
	Action:    scm("datadog-agent-action"),
	Installer: scm("Datadog Installer"),
	CoreAgent: scm("datadogagent"),
}

// All returns every Windows unit.
func All() []units.Unit {
	return units.Fields(Procmgr, SCM)
}

// Path returns the path of the rendered unit, relative to the gen directory.
func Path(u units.Unit) string {
	return path.Join("pm", "processes.d", u.Name)
}

// Render returns the unit with its placeholders replaced by installRoot and etcRoot.
func Render(u units.Unit, installRoot, etcRoot string) ([]byte, error) {
	content, err := generated.ReadFile(path.Join("gen", Path(u)))
	if err != nil {
		return nil, err
	}
	return []byte(strings.NewReplacer(
		InstallRootPlaceholder, yamlSingleQuoted(installRoot),
		EtcRootPlaceholder, yamlSingleQuoted(etcRoot),
	).Replace(string(content))), nil
}

// yamlSingleQuoted escapes a root for the single-quoted YAML scalars the templates put
// placeholders in.
func yamlSingleQuoted(root string) string {
	return strings.ReplaceAll(filepath.ToSlash(filepath.Clean(root)), "'", "''")
}

func procmgr(name, binary string) units.Unit {
	return units.Unit{Name: name + ".yaml", Manager: units.Procmgr, Template: name + "-windows.yaml.tmpl", Binary: binary}
}

func scm(name string) units.Unit {
	return units.Unit{Name: name, Manager: units.SCM}
}
