// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !windows

package impl

import "net"

// verifyCaller is a no-op on non-Linux platforms.
// SO_PEERCRED is a Linux-specific feature; on macOS (development host) the
// check is skipped.  Production deployments are Linux-only.
func verifyCaller(_ net.Conn) error {
	return nil
}
