// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package flags

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = path.Join(version.AgentPath, "etc/datadog.yaml")
	// DefaultSysProbeConfPath is set to empty since system-probe is not yet supported on darwin
	DefaultSysProbeConfPath = ""
)
