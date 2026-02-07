// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package flags

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// DefaultConfPath returns the default location of datadog.yaml
func DefaultConfPath() string {
	return filepath.Join(defaultpaths.GetDefaultConfPath(), "datadog.yaml")
}

// DefaultSysProbeConfPath returns the default location of system-probe.yaml
func DefaultSysProbeConfPath() string {
	return filepath.Join(defaultpaths.GetDefaultConfPath(), "system-probe.yaml")
}
