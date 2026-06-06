// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package setup

import (
	"path/filepath"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const (
	// defaultGuiPort is the default GUI port (-1 means disabled on Linux)
	defaultGuiPort = -1
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
	defaultSystemProbeBPFDir = filepath.Join(defaultpaths.GetInstallPath(), "embedded/share/system-probe/ebpf")
}

// FleetConfigOverride is a no-op on Linux
func FleetConfigOverride(_ pkgconfigmodel.Config) {
}
