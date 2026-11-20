// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxunits

import (
	"embed"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
)

//go:embed gen
var generated embed.FS

// Install is the way the Agent is installed, which decides the content and location of its units.
type Install string

const (
	OCI Install = "oci"
	DEB Install = "deb"
	RPM Install = "rpm"
)

// UnitsDir is the directory systemd units are installed into.
func (i Install) UnitsDir() string {
	switch i {
	case DEB:
		return "/lib/systemd/system"
	case RPM:
		return "/usr/lib/systemd/system"
	default:
		return "/etc/systemd/system"
	}
}

// Flavor selects which rendering of a systemd unit a host gets.
type Flavor struct {
	Install     Install
	AmbientCaps bool
}

func (f Flavor) dir() string {
	dir := "oci"
	if f.Install != OCI {
		dir = "deb"
	}
	if !f.AmbientCaps {
		dir += "nc"
	}
	return dir
}

// Path returns the path of the rendered unit, relative to the gen directory.
//
// Short directory names keep the paths within the Windows checkout path length limit.
func Path(u units.Unit, f Flavor, exp bool) string {
	manager := "sd"
	if u.Manager == units.Procmgr {
		manager = "pm"
	}
	if IsProcess(u) {
		return path.Join(manager, "processes.d", u.Name)
	}
	return path.Join(manager, f.dir(), u.Variant, u.InstalledName(exp))
}

// IsProcess reports whether u is a processes.d entry rather than a systemd unit.
func IsProcess(u units.Unit) bool {
	return strings.HasSuffix(u.Name, ".yaml")
}

// Content returns the rendered unit.
func Content(u units.Unit, f Flavor, exp bool) ([]byte, error) {
	return generated.ReadFile(path.Join("gen", Path(u, f, exp)))
}
