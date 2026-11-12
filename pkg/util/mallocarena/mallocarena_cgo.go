// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package mallocarena

/*
#include <stddef.h>

// M_ARENA_MAX from glibc's <malloc.h>, spelled out so this also builds against C libraries
// shipping no such header.
#define DD_M_ARENA_MAX (-8)

// Weak, so a C library without mallopt (musl) resolves the symbol to NULL rather than
// failing the link.
extern int mallopt(int param, int value) __attribute__((weak));

// Returns mallopt's result, or 0 (its failure value) when the symbol is absent.
static int ddSetArenaMax(int value) {
	if (mallopt == NULL) {
		return 0;
	}
	return mallopt(DD_M_ARENA_MAX, value);
}
*/
import "C"

// setArenaMax caps the arena count through mallopt. glibc honours M_ARENA_MAX after malloc
// initialises, so this bounds every arena not yet created — at startup, all but the first.
func setArenaMax(max int) error {
	if C.ddSetArenaMax(C.int(max)) != 1 {
		return errUnsupported
	}
	return nil
}
