// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mallocarena caps the number of per-thread arenas glibc's malloc may create.
//
// glibc hands each contended thread its own arena, up to 8*nproc, and only ever trims a
// secondary arena from the top of its heap. A process that churns large blocks from many
// threads fragments those heaps and settles at the sum of every arena's high-water mark
// rather than its live set.
//
// Importing this package is what applies the cap: on Linux with cgo it links an .init_array
// constructor that runs before the Go runtime starts, which is the only point early enough
// to matter. By the time main runs, a cgo binary has already opened an arena per startup
// thread, and glibc fixes its ceiling for good once more than arena_test (8) are open — past
// that a cap is ignored while mallopt still reports success. SetMax re-applies the limit for
// builds whose constructor did not run and reports what actually happened. Import it from
// main packages only; a library has no business changing the allocator under its callers.
//
// Other platforms need no equivalent. Windows has no per-thread arenas, services anything
// above the heap's ~508 KB VirtualMemoryThreshold straight from VirtualAlloc, and returns it
// on free; nor does it have a counterpart to glibc's dynamic M_MMAP_THRESHOLD, which is what
// migrates large blocks into arena heaps in the first place.
package mallocarena

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultMaxArenas is the limit applied when the environment does not pin one. The
// constructor hard-codes the same value; TestConstructorLimitMatchesDefault keeps them level.
const DefaultMaxArenas = 2

const (
	arenaMaxEnv     = "MALLOC_ARENA_MAX"
	tunablesEnv     = "GLIBC_TUNABLES"
	arenaMaxTunable = "glibc.malloc.arena_max"
)

// errUnsupported marks a build or C library with no arena limit to set.
var errUnsupported = errors.New("arena limit unsupported on this build")

// SetMax re-applies the arena cap for builds whose constructor did not run, and returns a
// summary of the outcome for logging. The returned error is non-nil only when the limit was
// expected to apply and did not.
//
// Call it early in main. It cannot undo arenas glibc has already handed out, so on its own it
// freezes the count rather than reaching limit — the constructor is what reaches it, and the
// summary says which of the two applied.
//
// An operator-provided MALLOC_ARENA_MAX or GLIBC_TUNABLES wins: glibc already applied it at
// startup, so overriding it here would discard a deliberate choice.
func SetMax(limit int) (string, error) {
	if override, ok := environmentLimit(); ok {
		return fmt.Sprintf("glibc malloc arena limit left to the environment (%s)", override), nil
	}

	switch err := setArenaMax(limit); {
	case errors.Is(err, errUnsupported):
		return "glibc malloc arena limit does not apply to this build", nil
	case err != nil:
		return fmt.Sprintf("could not cap glibc malloc arenas at %d: %v", limit, err), err
	}

	when := "after startup, so arenas already open are kept"
	if constructorApplied() {
		when = "before startup"
	}
	return fmt.Sprintf("capped glibc malloc arenas at %d (%s), override with %s", limit, when, arenaMaxEnv), nil
}

// environmentLimit reports an arena limit already pinned by the process environment.
func environmentLimit() (string, bool) {
	if v := os.Getenv(arenaMaxEnv); glibcAccepts(v) {
		return arenaMaxEnv + "=" + v, true
	}
	// GLIBC_TUNABLES holds colon-separated name=value pairs.
	for _, tunable := range strings.Split(os.Getenv(tunablesEnv), ":") {
		if name, value, found := strings.Cut(tunable, "="); found && name == arenaMaxTunable && glibcAccepts(value) {
			return tunablesEnv + " " + arenaMaxTunable + "=" + value, true
		}
	}
	return "", false
}

// glibcAccepts reports whether glibc would honour value as an arena limit. It discards
// anything below the tunable's minimum of 1 and falls back to 8*nproc, so reading such a
// value as an operator's choice would leave the process uncapped. This rejects what glibc
// plainly ignores rather than reproducing its parser.
func glibcAccepts(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1
}
