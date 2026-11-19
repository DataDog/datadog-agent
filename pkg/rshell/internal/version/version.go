// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package version exposes the build version of rshell.
//
// Agent builds stamp Version; external consumers use Go's module build info.
package version

import "runtime/debug"

const modulePath = "github.com/DataDog/datadog-agent/pkg/rshell"

// Version is the build version string.
var Version string

func init() {
	if Version == "" {
		Version = buildVersion()
	}
}

// buildVersion reads the module version when no Agent version was stamped.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return moduleVersion(info)
}

func moduleVersion(info *debug.BuildInfo) string {
	// When built as a standalone binary, check the main module.
	if info.Main.Path == modulePath && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	// When imported as a library, find ourselves in the dependency list.
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			if dep.Replace != nil {
				dep = dep.Replace
			}
			if dep.Version == "" || dep.Version == "(devel)" {
				return "dev"
			}
			return dep.Version
		}
	}
	return "dev"
}
