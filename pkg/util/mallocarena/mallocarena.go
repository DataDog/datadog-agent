// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mallocarena caps the number of per-thread arenas glibc's malloc may create.
//
// glibc hands each contended thread its own arena, up to 8*nproc, and never returns a
// secondary arena's memory to the OS. A process that churns large blocks from many threads
// therefore settles at the sum of every arena's high-water mark rather than its live set.
package mallocarena

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// DefaultMaxArenas is the limit applied when the environment does not pin one.
const DefaultMaxArenas = 2

const (
	arenaMaxEnv     = "MALLOC_ARENA_MAX"
	tunablesEnv     = "GLIBC_TUNABLES"
	arenaMaxTunable = "glibc.malloc.arena_max"
)

// errUnsupported marks a build or C library with no arena limit to set.
var errUnsupported = errors.New("arena limit unsupported on this build")

// SetMax caps glibc's arena count at max and returns a summary for logging. The returned
// error is non-nil only when the limit was expected to apply and did not.
//
// An operator-provided MALLOC_ARENA_MAX or GLIBC_TUNABLES wins: glibc already applied it at
// startup, so overriding it here would discard a deliberate choice. Call this before the
// process allocates from C, since it only bounds arenas that do not exist yet.
func SetMax(max int) (string, error) {
	if override, ok := environmentLimit(); ok {
		return fmt.Sprintf("glibc malloc arena limit left to the environment (%s)", override), nil
	}

	switch err := setArenaMax(max); {
	case errors.Is(err, errUnsupported):
		return "glibc malloc arena limit does not apply to this build", nil
	case err != nil:
		return fmt.Sprintf("could not cap glibc malloc arenas at %d: %v", max, err), err
	}
	return fmt.Sprintf("capped glibc malloc arenas at %d, override with %s", max, arenaMaxEnv), nil
}

// environmentLimit reports an arena limit already pinned by the process environment.
func environmentLimit() (string, bool) {
	if v := os.Getenv(arenaMaxEnv); v != "" {
		return arenaMaxEnv + "=" + v, true
	}
	// GLIBC_TUNABLES holds colon-separated name=value pairs.
	for _, tunable := range strings.Split(os.Getenv(tunablesEnv), ":") {
		if name, value, found := strings.Cut(tunable, "="); found && name == arenaMaxTunable {
			return tunablesEnv + " " + arenaMaxTunable + "=" + value, true
		}
	}
	return "", false
}
