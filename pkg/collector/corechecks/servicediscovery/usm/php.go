// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"strings"

	"slices"
)

const (
	artisanConsole = "artisan"
)

type phpDetector struct {
	ctx DetectionContext
}

func newPhpDetector(ctx DetectionContext) detector {
	return &phpDetector{ctx: ctx}
}

func (p phpDetector) detect(args []string) (ServiceMetadata, bool) {
	metadata := ServiceMetadata{}
	// Look for datadog.service (e.g., php -ddatadog.service=service_name OR php -d datadog.service=service_name)
	if index := slices.IndexFunc(args, func(arg string) bool { return strings.Contains(arg, "datadog.service=") }); index != -1 {
		split := strings.Split(args[index], "=")
		if len(split) == 2 {
			metadata.DDService = split[1]
		}
	}
	prevArgIsFlag := false
	for _, arg := range args {
		hasFlagPrefix := strings.HasPrefix(arg, "-")

		// If the previous argument was a flag, or is the current arg is a flag, skip the argument. Otherwise, process it.
		if !prevArgIsFlag && !hasFlagPrefix {
			basePath := removeFilePath(arg)
			if isRuneLetterAt(basePath, 0) && basePath == artisanConsole {
				metadata.SetNames(newLaravelParser(p.ctx).GetLaravelAppName(arg))
				return metadata, true
			}
		}

		includesAssignment := strings.ContainsRune(arg, '=')
		prevArgIsFlag = hasFlagPrefix && !includesAssignment
	}

	return metadata, false
}
