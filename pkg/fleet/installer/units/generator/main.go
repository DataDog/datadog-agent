// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main renders the units of every registry into their gen directories.
package main

import (
	"bytes"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	template "github.com/DataDog/datadog-agent/pkg/template/text"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/darwinunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/linuxunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/windowsunits"
)

//go:embed tmpl/*.tmpl
var templates embed.FS

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <units-dir>\n", os.Args[0])
		os.Exit(1)
	}
	if err := generate(os.Args[1]); err != nil {
		log.Fatalf("Failed to generate units: %v", err)
	}
}

type templateData struct {
	InstallDir       string
	EtcDir           string
	FleetPoliciesDir string
	PIDDir           string
	DDOTDir          string
	Stable           bool

	AmbiantCapabilitiesSupported bool
	Procmgr                      bool

	ProgramDir string
	RunDir     string
	LogDir     string
	AgentUser  string
	AgentGroup string
}

// file is a rendered unit, at its path relative to the units directory.
type file struct {
	path    string
	content []byte
}

func generate(unitsDir string) error {
	files, err := render()
	if err != nil {
		return err
	}
	for _, registry := range []string{"linuxunits", "darwinunits", "windowsunits"} {
		if err := os.RemoveAll(filepath.Join(unitsDir, registry, "gen")); err != nil {
			return err
		}
	}
	for _, f := range files {
		path := filepath.Join(unitsDir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.content, 0644); err != nil {
			return err
		}
	}
	return nil
}

func render() ([]file, error) {
	var files []file
	add := func(path string, u units.Unit, data templateData) error {
		content, err := renderTemplate(u.Template, data)
		if err != nil {
			return fmt.Errorf("%s: %w", u.Name, err)
		}
		files = append(files, file{path: path, content: content})
		return nil
	}

	for _, u := range withTemplate(linuxunits.All()) {
		if linuxunits.IsProcess(u) {
			if err := add(linuxPath(u, linuxunits.Flavor{}, false), u, templateData{}); err != nil {
				return nil, err
			}
			continue
		}
		for _, install := range []linuxunits.Install{linuxunits.OCI, linuxunits.DEB} {
			for _, ambientCaps := range []bool{true, false} {
				for _, exp := range []bool{false, true} {
					flavor := linuxunits.Flavor{Install: install, AmbientCaps: ambientCaps}
					if err := add(linuxPath(u, flavor, exp), u, linuxData(u, flavor, exp)); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	for _, u := range withTemplate(darwinunits.All()) {
		if err := add(filepath.Join("darwinunits", "gen", darwinunits.Path(u)), u, darwinData); err != nil {
			return nil, err
		}
	}

	for _, u := range withTemplate(windowsunits.All()) {
		if err := add(filepath.Join("windowsunits", "gen", windowsunits.Path(u)), u, windowsData); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func linuxPath(u units.Unit, f linuxunits.Flavor, exp bool) string {
	return filepath.Join("linuxunits", "gen", linuxunits.Path(u, f, exp))
}

func linuxData(u units.Unit, f linuxunits.Flavor, exp bool) templateData {
	pkg := "datadog-agent"
	if u == linuxunits.Systemd.DDOTStandalone {
		pkg = "datadog-agent-ddot"
	}
	installDir := "/opt/datadog-agent"
	if f.Install == linuxunits.OCI {
		installDir = "/opt/datadog-packages/" + pkg + "/stable"
		if exp {
			installDir = "/opt/datadog-packages/" + pkg + "/experiment"
		}
	}
	ddotDir := installDir + "/ext/ddot"
	if u == linuxunits.Systemd.DDOTStandalone {
		ddotDir = installDir
	}
	etcDir := "/etc/datadog-agent"
	if exp {
		etcDir = "/etc/datadog-agent-exp"
	}
	return templateData{
		InstallDir:                   installDir,
		EtcDir:                       etcDir,
		FleetPoliciesDir:             etcDir + "/managed/datadog-agent/stable",
		PIDDir:                       installDir,
		DDOTDir:                      ddotDir,
		Stable:                       !exp,
		AmbiantCapabilitiesSupported: f.AmbientCaps,
		Procmgr:                      u.Manager == units.Procmgr,
	}
}

// macOS has a single install root holding the binaries alongside etc, run and logs.
var darwinData = templateData{
	InstallDir: "/opt/datadog-agent",
	ProgramDir: "/opt/datadog-agent",
	EtcDir:     "/opt/datadog-agent/etc",
	RunDir:     "/opt/datadog-agent/run",
	LogDir:     "/opt/datadog-agent/logs",
	AgentUser:  "_dd-agent",
	AgentGroup: "daemon",
	Stable:     true,
}

// Windows roots are only known at install time, which replaces the placeholders.
var windowsData = templateData{
	InstallDir: windowsunits.InstallRootPlaceholder,
	EtcDir:     windowsunits.EtcRootPlaceholder,
	Stable:     true,
}

func withTemplate(all []units.Unit) []units.Unit {
	var rendered []units.Unit
	for _, u := range all {
		if u.Template != "" {
			rendered = append(rendered, u)
		}
	}
	return rendered
}

func renderTemplate(name string, data templateData) ([]byte, error) {
	tmpl, err := template.ParseFS(templates, "tmpl/"+name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
