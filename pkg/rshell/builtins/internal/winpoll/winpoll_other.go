// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package winpoll provides a non-consuming readability probe for Windows
// file handles. On non-Windows platforms it is a no-op stub: callers
// should use the platform-native poll(2) path instead.
package winpoll

// PollNonConsuming returns supported=false on non-Windows platforms.
// Callers should not depend on this stub for readiness checking — Unix
// builds use the poll(2) path in builtins/read/poll_unix.go directly.
func PollNonConsuming(fd uintptr) (available, supported bool) {
	return false, false
}
