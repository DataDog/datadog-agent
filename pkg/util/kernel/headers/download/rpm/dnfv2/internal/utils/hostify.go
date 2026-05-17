// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func rawHostJoin(envName, defaultValue string, parts ...string) string {
	if len(parts) == 0 {
		return ""
	}

	first := parts[0]
	hostPath := os.Getenv(envName)
	if hostPath == "" || !strings.HasPrefix(first, defaultValue) {
		return filepath.Join(parts...)
	}

	first = strings.TrimPrefix(first, defaultValue)
	newParts := make([]string, len(parts)+1)
	newParts[0] = hostPath
	newParts[1] = first
	if len(parts) > 1 {
		copy(newParts[2:], parts[1:])
	}
	return filepath.Join(newParts...)
}

// HostEtcJoin joins `parts` together, replacing the `/etc` prefix with the HOST_ETC environment variable, if set.
// If `parts` does not begin with `/etc`, then they are joined without modification.
func HostEtcJoin(parts ...string) string {
	return rawHostJoin("HOST_ETC", "/etc", parts...)
}
