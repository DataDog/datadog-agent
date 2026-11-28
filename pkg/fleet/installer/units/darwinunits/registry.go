// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package darwinunits is the registry of the macOS service units.
package darwinunits

import (
	"embed"
	"path"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
)

//go:embed gen
var generated embed.FS

// Launchd holds the LaunchDaemons, named by label.
var Launchd = struct {
	Procmgr, Agent, Sysprobe, DataPlane, Installer units.Unit
}{
	Procmgr:   launchd("com.datadoghq.procmgr"),
	Agent:     launchd("com.datadoghq.agent"),
	Sysprobe:  launchd("com.datadoghq.sysprobe"),
	DataPlane: launchd("com.datadoghq.data-plane"),
	Installer: launchd("com.datadoghq.installer"),
}

// LaunchdGUI holds the per-user LaunchAgents, installed from the .plist.example files of the
// Agent configuration directory.
var LaunchdGUI = struct {
	GUI, Monitor, OldMonitor units.Unit
}{
	GUI:        units.Unit{Name: "com.datadoghq.gui", Manager: units.LaunchdGUI},
	Monitor:    units.Unit{Name: "com.datadoghq.ai-usage-agent.desktop-monitor", Manager: units.LaunchdGUI},
	OldMonitor: units.Unit{Name: "com.datadoghq.ai-prompt-logger.desktop-monitor", Manager: units.LaunchdGUI},
}

// Procmgr holds the processes.d entries.
var Procmgr = struct {
	Sysprobe units.Unit
}{
	Sysprobe: units.Unit{Name: "datadog-agent-sysprobe.yaml", Manager: units.Procmgr, Template: "datadog-agent-sysprobe-darwin.yaml.tmpl"},
}

// All returns every macOS unit.
func All() []units.Unit {
	return units.Fields(Launchd, LaunchdGUI, Procmgr)
}

// Path returns the path of the rendered unit, relative to the gen directory.
func Path(u units.Unit) string {
	if u.Manager == units.Procmgr {
		return path.Join("pm", "processes.d", u.Name)
	}
	return path.Join("launchd", u.Name+".plist")
}

// Content returns the rendered unit.
func Content(u units.Unit) ([]byte, error) {
	return generated.ReadFile(path.Join("gen", Path(u)))
}

func launchd(label string) units.Unit {
	return units.Unit{Name: label, Manager: units.Launchd, Template: label + ".plist.tmpl"}
}
