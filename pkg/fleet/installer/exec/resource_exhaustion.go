// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"bytes"
	"errors"
)

// ErrResourceExhausted indicates that the installer subprocess crashed at Go-runtime bootstrap
// because the host ran out of memory, threads, or paging capacity, rather than failing for some
// other reason. Use errors.Is(err, ErrResourceExhausted) to distinguish this case from other
// "run failed" errors.
var ErrResourceExhausted = errors.New("installer subprocess crashed due to host resource exhaustion (memory/thread limit)")

// fatalErrorPrefix is the exact prefix Go's runtime.throw()/runtime.fatal() always emit,
// immediately followed on the same line by the message passed to throw/fatal (see
// runtime/panic.go, printindented). We anchor the runtime-internal signatures below on this
// prefix so we only match genuine Go-runtime crashes, not incidental occurrences of otherwise
// fairly generic phrases (like "out of memory") elsewhere in a config value, path, or argument
// that happened to get echoed into the subprocess's combined output.
const fatalErrorPrefix = "fatal error: "

// resourceExhaustionFatalMessages are exact runtime.throw() message strings that Go's runtime
// prints as "fatal error: <msg>" when it cannot grow the heap, map new arenas, or spin up more
// OS threads. Each entry below is cited against the runtime source (Go 1.26, this repo's pinned
// toolchain) so we don't match on invented wording.
var resourceExhaustionFatalMessages = []string{
	// runtime/mpagealloc.go: (*pageAlloc).grow
	"pageAlloc: out of memory",
	// runtime/malloc.go: (*mheap).sysAlloc
	"out of memory allocating heap arena metadata",
	// runtime/malloc.go: (*mheap).sysAlloc
	"out of memory allocating heap arena map",
	// runtime/malloc.go: (*mheap).sysAlloc
	"out of memory allocating allArenas",
	// runtime/mem_linux.go, mem_bsd.go, mem_darwin.go: sysMapOS/sysUsedOS
	"runtime: out of memory",
	// runtime/mem_windows.go: sysUsedOS
	"out of memory",
	// runtime/proc.go: checkmcount
	"thread exhaustion",
}

// resourceExhaustionDiagnosticSignatures are runtime print()-level diagnostic lines, or plain OS
// errno/formatted-error text, that accompany or stand in for a resource-exhaustion crash but are
// not themselves preceded by the "fatal error: " prefix (either because they come from a
// runtime print() call that precedes a separate, less-specific throw(), or because they're a
// syscall errno string / OS-formatted message rather than a runtime panic at all). These are
// matched as plain substrings since there's no fixed prefix to anchor on; each is still a
// fairly specific phrase, unlikely to appear incidentally in unrelated output.
var resourceExhaustionDiagnosticSignatures = []string{
	// runtime/os_linux.go, os_windows.go: newosproc
	"failed to create new OS thread",
	// runtime/proc.go: checkmcount
	"-thread limit",
	// runtime/mem_windows.go: sysUsedOS
	"VirtualAlloc",
	// runtime/mem_linux.go: sysAllocOS
	"too much locked memory",
	// syscall/zerrors_*.go: strerror(ENOMEM)
	"cannot allocate memory",
	// Windows FormatMessage text for ERROR_COMMITMENT_LIMIT (1455)
	"paging file is too small",
}

// isResourceExhaustionCrash returns true if the given subprocess output (its captured
// stderr/combined output only — callers must not concatenate unrelated data into the same
// buffer) matches a known Go-runtime crash signature for host memory/thread/paging exhaustion.
//
// Matching is a small, fixed number of substring scans over the buffer (no repeated
// re-scanning), so cost is linear in the size of output regardless of how large it is. If
// output was truncated mid-message, a signature landing exactly on the truncation boundary can
// go undetected; that is an intentional, accepted trade-off rather than a hidden bug, since we
// have no reliable way to distinguish a genuinely different error from a truncated match.
func isResourceExhaustionCrash(output []byte) bool {
	if len(output) == 0 {
		return false
	}
	for _, msg := range resourceExhaustionFatalMessages {
		if bytes.Contains(output, []byte(fatalErrorPrefix+msg)) {
			return true
		}
	}
	for _, sig := range resourceExhaustionDiagnosticSignatures {
		if bytes.Contains(output, []byte(sig)) {
			return true
		}
	}
	return false
}
