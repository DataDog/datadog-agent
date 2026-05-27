// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm

package http

// txCmdline is a no-op on Windows; the temporary diagnostics in statkeeper.go
// only need a real lookup on Linux where /proc/<pid>/cmdline is available.
func txCmdline(_ Transaction) string {
	return ""
}
