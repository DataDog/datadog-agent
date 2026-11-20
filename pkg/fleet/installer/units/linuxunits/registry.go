// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package linuxunits is the registry of the Linux service units.
package linuxunits

import "github.com/DataDog/datadog-agent/pkg/fleet/installer/units"

// Systemd holds the systemd units. They are shared by the systemd and procmgr service managers
// unless Procmgr holds a unit with the same name.
var Systemd = struct {
	Agent, Installer, Trace, Process, Sysprobe, Security, DataPlane, Action, DDOT units.Unit
	RshellService, RshellSocket                                                   units.Unit
	// DDOTStandalone is the unit of the standalone datadog-agent-ddot package.
	DDOTStandalone units.Unit
	// LegacyInstaller and LegacyProcmgrd are units older installers wrote.
	LegacyInstaller, LegacyProcmgrd units.Unit
}{
	Agent:           systemd("datadog-agent.service"),
	Installer:       systemd("datadog-agent-installer.service"),
	Trace:           systemd("datadog-agent-trace.service"),
	Process:         systemd("datadog-agent-process.service"),
	Sysprobe:        systemd("datadog-agent-sysprobe.service"),
	Security:        systemd("datadog-agent-security.service"),
	DataPlane:       systemd("datadog-agent-data-plane.service"),
	Action:          systemd("datadog-agent-action.service"),
	DDOT:            systemd("datadog-agent-ddot.service"),
	RshellService:   systemd("datadog-agent-rshell-privileged.service"),
	RshellSocket:    systemd("datadog-agent-rshell-privileged.socket"),
	DDOTStandalone:  units.Unit{Name: "datadog-agent-ddot.service", Manager: units.Systemd, Template: "datadog-agent-ddot.service.tmpl", Variant: "ddot"},
	LegacyInstaller: units.Unit{Name: "datadog-installer.service", Manager: units.Systemd},
	LegacyProcmgrd:  units.Unit{Name: "datadog-agent-procmgrd.service", Manager: units.Systemd},
}

// Procmgr holds the units specific to the procmgr service manager: the systemd units differing
// from Systemd and the processes.d entries.
var Procmgr = struct {
	Agent, Procmgr                   units.Unit
	DDOT, ActionExecutor, ParControl units.Unit
}{
	Agent:          procmgr("datadog-agent.service"),
	Procmgr:        procmgr("datadog-agent-procmgr.service"),
	DDOT:           procmgr("datadog-agent-ddot.yaml"),
	ActionExecutor: procmgr("datadog-agent-action-executor.yaml"),
	ParControl:     procmgr("datadog-agent-par-control.yaml"),
}

// Upstart holds the upstart jobs, shipped by the deb/rpm packages.
var Upstart = struct {
	Agent, Trace, Process, Sysprobe, Security, DataPlane, Procmgr, Action, DDOT units.Unit
}{
	Agent:     script(units.Upstart, "datadog-agent"),
	Trace:     script(units.Upstart, "datadog-agent-trace"),
	Process:   script(units.Upstart, "datadog-agent-process"),
	Sysprobe:  script(units.Upstart, "datadog-agent-sysprobe"),
	Security:  script(units.Upstart, "datadog-agent-security"),
	DataPlane: script(units.Upstart, "datadog-agent-data-plane"),
	Procmgr:   script(units.Upstart, "datadog-agent-procmgr"),
	Action:    script(units.Upstart, "datadog-agent-action"),
	DDOT:      script(units.Upstart, "datadog-agent-ddot"),
}

// Sysvinit holds the sysvinit scripts, shipped by the deb package.
var Sysvinit = struct {
	Agent, Trace, Process, Security, DataPlane, Procmgr, Action, DDOT units.Unit
}{
	Agent:     script(units.Sysvinit, "datadog-agent"),
	Trace:     script(units.Sysvinit, "datadog-agent-trace"),
	Process:   script(units.Sysvinit, "datadog-agent-process"),
	Security:  script(units.Sysvinit, "datadog-agent-security"),
	DataPlane: script(units.Sysvinit, "datadog-agent-data-plane"),
	Procmgr:   script(units.Sysvinit, "datadog-agent-procmgr"),
	Action:    script(units.Sysvinit, "datadog-agent-action"),
	DDOT:      script(units.Sysvinit, "datadog-agent-ddot"),
}

// All returns every Linux unit.
func All() []units.Unit {
	return units.Fields(Systemd, Procmgr, Upstart, Sysvinit)
}

func systemd(name string) units.Unit {
	return units.Unit{Name: name, Manager: units.Systemd, Template: name + ".tmpl"}
}

func procmgr(name string) units.Unit {
	return units.Unit{Name: name, Manager: units.Procmgr, Template: name + ".tmpl"}
}

func script(manager units.Manager, name string) units.Unit {
	return units.Unit{Name: name, Manager: manager}
}
