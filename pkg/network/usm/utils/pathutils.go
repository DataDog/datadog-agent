// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package utils

import (
	"os"
	"time"
)

// ResolveSymlink returns the target path of the symbolic link specified by linkPath.
// If the link target is relative, it returns the relative path without resolving it to an absolute path.
// If the link target cannot be resolved immediately, it retries for a short period.
func ResolveSymlink(linkPath string) (string, error) {
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		// If Readlink fails, retry for up to 10 milliseconds in case of transient issues.
		end := time.Now().Add(10 * time.Millisecond)
		for end.After(time.Now()) {
			targetPath, err = os.Readlink(linkPath)
			if err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}

	if err != nil {
		// we can't access to the binary path here (pid probably ended already)
		// there are not much we can do, and we don't want to flood the logs
		return "", err
	}
	return targetPath, nil
}
