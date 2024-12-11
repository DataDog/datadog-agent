// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flags

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog\\datadog.yaml"
	// DefaultSysProbeConfPath points to the location of system-probe.yaml
	DefaultSysProbeConfPath = "c:\\programdata\\datadog\\system-probe.yaml"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfPath = filepath.Join(pd, "datadog.yaml")
		DefaultSysProbeConfPath = filepath.Join(pd, "system-probe.yaml")
	}
}
