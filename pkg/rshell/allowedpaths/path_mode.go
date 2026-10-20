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
