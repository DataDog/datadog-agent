// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package builtins

// isBrokenPipePlatform always reports false on non-Windows platforms: Unix
// broken-pipe writes are already covered by syscall.EPIPE in IsBrokenPipe,
// and the Windows-specific numeric errno values (109, 232) must never be
// checked here — errno 109 is ETOOMANYREFS on Linux, a real, unrelated
// error that must not be misreported as a closed pipe.
func isBrokenPipePlatform(err error) bool {
	return false
}
