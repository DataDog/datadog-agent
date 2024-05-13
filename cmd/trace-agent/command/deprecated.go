// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/cli"
)

// FixDeprecatedFlags transforms args so that they are conforming to the new CLI structure:
// - replace non-posix flags posix flags
// - replace deprecated flags with their command equivalents
// - display warnings when deprecated flags are encountered
func FixDeprecatedFlags(args []string, w io.Writer) []string {
	var (
		replacePid        = cli.ReplaceFlagExact("--pidfile")
		replaceInfo       = cli.ReplaceFlagExact("info")
		replaceVersion    = cli.ReplaceFlagExact("version")
		replaceCPUProfile = cli.ReplaceFlagExact("--cpu-profile")
		replaceMemProfile = cli.ReplaceFlagExact("--mem-profile")
	)

	deprecatedFlags := map[string]cli.ReplaceFlagFunc{
		// Global flags
		"-config":   cli.ReplaceFlagPosix,
		"-version":  replaceVersion,
		"--version": replaceVersion,
		"-info":     replaceInfo,
		"--info":    replaceInfo,
		"-pid":      replacePid,
		// Profiling flags
		"-cpuprofile": replaceCPUProfile,
		"-memprofile": replaceMemProfile,
		// Windows flags
		"-start-service": cli.ReplaceFlagPosix,
		"-stop-service":  cli.ReplaceFlagPosix,
		"-foreground":    cli.ReplaceFlagPosix,
	}

	return cli.FixDeprecatedFlags(args, w, deprecatedFlags)

}
