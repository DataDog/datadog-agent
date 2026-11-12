// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package coredump

/*
#include <datadog_agent_rtloader.h>
#include <signal.h>
#include <stdint.h>
#include <stdlib.h>

static uintptr_t action_handler(const struct sigaction *action) {
    if (action->sa_flags & SA_SIGINFO) {
        return (uintptr_t)action->sa_sigaction;
    }
    return (uintptr_t)action->sa_handler;
}

static int enable_core_dumps_and_check_sigsegv(char **error) {
    struct sigaction before;
    struct sigaction after;
    if (sigaction(SIGSEGV, NULL, &before) != 0) {
        return -1;
    }
    // The deprecated stacktrace flag must remain a no-op for ABI compatibility.
    if (handle_crashes(0, 1, error) == 0) {
        return 0;
    }
    if (sigaction(SIGSEGV, NULL, &after) != 0) {
        return -1;
    }
    if (before.sa_flags != after.sa_flags || action_handler(&before) != action_handler(&after)) {
        return -2;
    }
    if (enable_core_dumps(error) == 0) {
        return 0;
    }
    if (sigaction(SIGSEGV, NULL, &after) != 0) {
        return -1;
    }
    if (before.sa_flags != after.sa_flags || action_handler(&before) != action_handler(&after)) {
        return -2;
    }
    return 1;
}
*/
import "C"

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"unsafe"
)

func recoverNilDereference() (recovered any) {
	defer func() { recovered = recover() }()
	var pointer *int
	_ = *pointer
	return nil
}

func TestEnableCoreDumpsPreservesGoSignalHandling(t *testing.T) {
	if os.Getenv("RTLOADER_CORE_DUMP_CHILD") == "1" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var cError *C.char
		result := C.enable_core_dumps_and_check_sigsegv(&cError)
		if cError != nil {
			defer C.free(unsafe.Pointer(cError))
		}
		if result == 0 {
			t.Fatalf("enable_core_dumps failed: %s", C.GoString(cError))
		}
		if result < 0 {
			t.Fatalf("SIGSEGV disposition changed or could not be read: %d", result)
		}

		if recovered := recoverNilDereference(); recovered == nil {
			t.Fatal("first nil dereference was not recovered")
		}
		if recovered := recoverNilDereference(); recovered == nil {
			t.Fatal("second nil dereference was not recovered")
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=TestEnableCoreDumpsPreservesGoSignalHandling", "-test.v")
	command.Env = append(os.Environ(), "RTLOADER_CORE_DUMP_CHILD=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed: %v\n%s", err, output)
	}
	if bytes.Contains(output, []byte("HANDLER CAUGHT")) {
		t.Fatalf("rtloader installed a SIGSEGV handler:\n%s", output)
	}
}
