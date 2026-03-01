// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"os"
	"path/filepath"
	"strings"
)

// getHostPrefix returns "/host" when running inside a container (where host
// directories are bind-mounted under /host), or "" when running directly on
// the host.
func getHostPrefix() string {
	if _, err := os.Stat("/host/proc"); err == nil {
		return "/host"
	}
	return ""
}

// toHostPath converts a host-relative path (e.g. "/var/log") into the
// corresponding container path (e.g. "/host/var/log").
func toHostPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	return filepath.Join(prefix, path)
}

// toOutputPath strips the host prefix from a container-local path so the
// caller sees the original host-relative path.
func toOutputPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	return strings.TrimPrefix(path, prefix)
}
