// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package dummymodeimpl

import (
	"os"
	"syscall"
)

// requestStop asks ADP to shut down gracefully.
//
// SIGINT, not SIGTERM: ADP (agent-data-plane 1.4.0) only installs a handler for SIGINT —
// /proc/<pid>/status reports SigCgt without bit 15 — so SIGTERM falls through to the default
// disposition and kills it outright, losing the graceful shutdown the pre-flight exists to
// exercise. Verified against the released binary, not inferred.
func requestStop(p *os.Process) error {
	return p.Signal(syscall.SIGINT)
}
