// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package installer

import (
	"syscall"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
)

// setInstallerUmask sets umask 0022 to override any inherited umask.
// Files are created with at most 644 permissions
// Dirs are created with at most 755 permissions
// Any file that requires more permissive permissions should be set explicitly
func setInstallerUmask(span ddtrace.Span) {
	oldmask := syscall.Umask(0022)
	span.SetTag("inherited_umask", oldmask)
}
