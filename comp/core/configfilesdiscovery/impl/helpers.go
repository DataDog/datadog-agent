// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"path"
	"strings"
)

func commandlineArgs(commandline TargetCommandline) []string {
	return unwrapShellCommandline(commandline.Args)
}

func unwrapShellCommandline(args []string) []string {
	if len(args) < 3 || !isShellExecutable(args[0]) || args[1] != "-c" {
		return args
	}
	return strings.Fields(args[2])
}

func isShellExecutable(arg string) bool {
	switch path.Base(arg) {
	case "sh", "bash", "dash", "ash", "zsh":
		return true
	default:
		return false
	}
}
