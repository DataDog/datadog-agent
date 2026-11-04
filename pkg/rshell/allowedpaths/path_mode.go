// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import "strings"

type pathMode uint8

const (
	pathModeReadOnly pathMode = iota
	pathModeReadWrite
)

// parseAllowedPathMode strips a terminal ":ro" or ":rw" access suffix. The
// suffix is always access metadata, never filename text: a path whose real
// name ends in one of these suffixes cannot be configured. Parsing never
// consults the filesystem, so files a script creates cannot change which roots
// a later sandbox opens or their access modes.
func parseAllowedPathMode(path string) (string, pathMode) {
	path, mode, _ := splitAllowedPathMode(path)
	return path, mode
}

func splitAllowedPathMode(path string) (string, pathMode, bool) {
	for _, suffix := range []struct {
		text string
		mode pathMode
	}{
		{text: ":ro", mode: pathModeReadOnly},
		{text: ":rw", mode: pathModeReadWrite},
	} {
		if strings.HasSuffix(path, suffix.text) && len(path) > len(suffix.text) {
			return path[:len(path)-len(suffix.text)], suffix.mode, true
		}
	}
	return path, pathModeReadOnly, false
}
