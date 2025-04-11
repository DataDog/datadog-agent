// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	agentPackage    = "datadog-agent"
	installerCaller = "installer"
)

var (
	// StablePath is the path to the stable agent installation
	StablePath = filepath.Join(paths.PackagesPath, agentPackage, "stable")
	// ExperimentPath is the path to the experimental agent installation
	ExperimentPath = filepath.Join(paths.PackagesPath, agentPackage, "experiment")
)
