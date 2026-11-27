// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package mallocarena

/*
#include <stdlib.h>
#include <string.h>

// M_ARENA_MAX from glibc's <malloc.h>, spelled out so this also builds against C libraries
// shipping no such header.
#define DD_M_ARENA_MAX (-8)

// Must match DefaultMaxArenas: the constructor runs before Go and cannot read a Go constant.
#define DD_DEFAULT_MAX_ARENAS 2

// Weak, so a C library without mallopt (musl) resolves the symbol to NULL rather than
// failing the link.
extern int mallopt(int param, int value) __attribute__((weak));

// What ddCapArenas did, reported once Go has a logger. 1 capped, 0 mallopt refused,
// -1 no mallopt, -2 never ran, -3 stood down because the environment names a limit.
static int ddCtorOutcome = -2;

// ddEnvNamesArenaMax reports whether the environment mentions an arena limit at all. The
// check is deliberately coarser than the Go one: standing down here only costs the arenas
// the runtime opens before main, which SetMax's fallback still bounds.
static int ddEnvNamesArenaMax(void) {
	const char *v = getenv("MALLOC_ARENA_MAX");
	if (v != NULL && v[0] != '\0') {
		return 1;
	}
	const char *tunables = getenv("GLIBC_TUNABLES");
	return tunables != NULL && strstr(tunables, "glibc.malloc.arena_max=") != NULL;
}

// Caps arenas from .init_array, which runs before __libc_start_main enters main and so before
// the Go runtime starts the threads that would each open an arena of their own. Capping from
// main instead only freezes the count startup already reached.
__attribute__((constructor)) static void ddCapArenas(void) {
	if (ddEnvNamesArenaMax()) {
		ddCtorOutcome = -3;
		return;
	}
	if (mallopt == NULL) {
		ddCtorOutcome = -1;
		return;
	}
	ddCtorOutcome = mallopt(DD_M_ARENA_MAX, DD_DEFAULT_MAX_ARENAS);
}

static int ddConstructorOutcome(void) { return ddCtorOutcome; }

// Returns mallopt's own result, 1 on success and 0 on failure, or -1 if there is no mallopt.
static int ddSetArenaMax(int value) {
	if (mallopt == NULL) {
		return -1;
	}
	return mallopt(DD_M_ARENA_MAX, value);
}
*/
import "C"

import "fmt"

// constructorLimit is the limit compiled into the constructor, checked against
// DefaultMaxArenas by the tests.
const constructorLimit = C.DD_DEFAULT_MAX_ARENAS

// constructorApplied reports whether the .init_array constructor capped arenas before the Go
// runtime started. That is the difference between reaching the limit and merely freezing the
// arena count that startup reached.
func constructorApplied() bool {
	return C.ddConstructorOutcome() == 1
}

// setArenaMax caps the arena count through mallopt. It is the fallback for builds whose
// constructor did not run, and a no-op repeat when it did.
func setArenaMax(limit int) error {
	switch rc := C.ddSetArenaMax(C.int(limit)); rc {
	case 1:
		return nil
	case -1:
		return errUnsupported
	default:
		return fmt.Errorf("mallopt(M_ARENA_MAX, %d) returned %d", limit, rc)
	}
}
