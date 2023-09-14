// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activitytree

// ExtractFirstParent extracts first parent
func ExtractFirstParent(path string) (string, int) {
	if len(path) == 0 {
		return "", 0
	}
	if path == "/" {
		return "", 0
	}

	var add int
	if path[0] == '/' {
		path = path[1:]
		add = 1
	}

	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			return path[0:i], i + add
		}
	}

	return path, len(path) + add
}
