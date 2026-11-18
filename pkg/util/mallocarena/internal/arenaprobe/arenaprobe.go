// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

// Package arenaprobe creates and counts glibc malloc arenas so that mallocarena's tests can
// assert the cap took effect. It is internal and imported only from tests, which keeps
// malloc_info out of every shipped binary.
package arenaprobe

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Weak, like mallopt: malloc_info is a glibc extension and musl must still link.
extern int malloc_info(int options, FILE *stream) __attribute__((weak));

// ddArenaCount returns the arena count malloc_info reports, or -1 when it cannot be read.
static int ddArenaCount(void) {
	if (malloc_info == NULL) {
		return -1;
	}

	char *buf = NULL;
	size_t len = 0;
	FILE *out = open_memstream(&buf, &len);
	if (out == NULL) {
		return -1;
	}
	int rc = malloc_info(0, out);
	fclose(out);

	int arenas = -1;
	if (rc == 0) {
		arenas = 0;
		// malloc_info emits one <heap nr="N"> element per arena.
		for (const char *p = buf; (p = strstr(p, "<heap nr=")) != NULL; p++) {
			arenas++;
		}
	}
	free(buf);
	return arenas;
}

// ddChurn allocates and frees on the calling thread.
static void ddChurn(int iterations, size_t size) {
	for (int i = 0; i < iterations; i++) {
		void *p = malloc(size);
		if (p == NULL) {
			return;
		}
		memset(p, 1, size < 4096 ? size : 4096);
		free(p);
	}
}
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
)

// Count returns the number of malloc arenas the process currently holds.
func Count() (int, error) {
	n := int(C.ddArenaCount())
	if n < 0 {
		return 0, errors.New("malloc_info unavailable: not glibc")
	}
	return n, nil
}

// Churn allocates from C on the given number of locked OS threads at once. glibc only hands
// out a secondary arena to a thread that finds the arena it wants locked, so the allocations
// have to overlap; the threads are released together for that reason.
func Churn(threads, iterations, size int) {
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(threads)

	for range threads {
		go func() {
			defer done.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			start.Wait()
			C.ddChurn(C.int(iterations), C.size_t(size))
		}()
	}

	start.Done()
	done.Wait()
}
